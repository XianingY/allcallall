// Package async provides a bounded worker pool for background jobs.
//
// Motivation: several hot paths used to spawn a bare `go func()` per event
// (content moderation per message, RAG chunk indexing per chunk). Under load
// that produces unbounded concurrent goroutines, each holding an outbound
// connection, and failures were silently discarded. This pool gives every such
// path the same guarantees: bounded concurrency, bounded retries, a dead-letter
// callback for terminal failures, and queue-depth metrics to see backpressure.
package async

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/metrics"
)

// ErrQueueFull is returned by Submit when the queue is at capacity. Callers must
// decide whether to drop (and count) or fall back to synchronous execution; the
// pool never spawns an unbounded goroutine as a relief valve.
var ErrQueueFull = errors.New("async: queue is full")

// Job is a unit of background work.
type Job struct {
	// Kind groups jobs for metrics and dead-letter routing, e.g. "moderation".
	Kind string
	// Key identifies the job (used for logs and dead-letter dedup).
	Key string
	// Run performs the work. It must respect context cancellation.
	Run func(ctx context.Context) error
}

// Result describes the outcome of an execution attempt.
type Result struct {
	Job     Job
	Attempt int
	Err     error
}

// Options configures a Pool.
type Options struct {
	// Name identifies the pool in logs and metrics.
	Name string
	// Workers is the maximum concurrent in-flight jobs.
	Workers int
	// QueueSize is the buffered queue depth. Submit fails with ErrQueueFull
	// once full, so backpressure is observable instead of unbounded.
	QueueSize int
	// MaxAttempts is the total number of tries (>=1).
	MaxAttempts int
	// RetryDelay is the delay between attempts.
	RetryDelay time.Duration
	// JobTimeout bounds a single attempt.
	JobTimeout time.Duration
	// Logger and Metrics are optional observability sinks.
	Logger  zerolog.Logger
	Metrics metrics.Recorder
	// OnDeadLetter is invoked when a job exhausts all attempts. It must be
	// non-blocking-ish; it runs on a worker goroutine.
	OnDeadLetter func(ctx context.Context, r Result)
}

// Pool is a bounded, observable background worker pool.
type Pool struct {
	opts    Options
	jobs    chan Job
	logger  zerolog.Logger
	metrics metrics.Recorder

	mu       sync.Mutex
	queued   int
	inflight int
	done     bool

	wg sync.WaitGroup
}

// NewPool builds a pool with sane defaults for any zero-valued option.
func NewPool(opts Options) *Pool {
	if opts.Name == "" {
		opts.Name = "default"
	}
	if opts.Workers <= 0 {
		opts.Workers = 8
	}
	if opts.QueueSize <= 0 {
		opts.QueueSize = 256
	}
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 3
	}
	if opts.RetryDelay <= 0 {
		opts.RetryDelay = time.Second
	}
	if opts.JobTimeout <= 0 {
		opts.JobTimeout = 15 * time.Second
	}
	if opts.Metrics == nil {
		opts.Metrics = metrics.NewCounterStore()
	}
	return &Pool{
		opts:    opts,
		jobs:    make(chan Job, opts.QueueSize),
		logger:  opts.Logger,
		metrics: opts.Metrics,
	}
}

// Start launches the workers. It returns immediately; workers exit when ctx is
// cancelled or Close is called and the queue drains.
func (p *Pool) Start(ctx context.Context) {
	for i := 0; i < p.opts.Workers; i++ {
		p.wg.Add(1)
		go p.worker(ctx, i)
	}
}

// Submit enqueues a job. It never blocks and never spawns a goroutine: when the
// queue is full it returns ErrQueueFull so the caller can count the drop.
func (p *Pool) Submit(_ context.Context, job Job) error {
	if job.Run == nil {
		return errors.New("async: job has no Run function")
	}
	if job.Kind == "" {
		job.Kind = "unknown"
	}
	if job.Key == "" {
		job.Key = uuid.NewString()
	}

	p.mu.Lock()
	if p.done {
		p.mu.Unlock()
		return errors.New("async: pool is closed")
	}
	p.mu.Unlock()

	select {
	case p.jobs <- job:
		p.mu.Lock()
		p.queued++
		p.mu.Unlock()
		p.metrics.Set("async_queue_depth_"+p.opts.Name, int64(p.queueLen()))
		return nil
	default:
		p.metrics.Inc("async_dropped_total")
		p.logger.Warn().
			Str("pool", p.opts.Name).
			Str("kind", job.Kind).
			Str("key", job.Key).
			Msg("async queue full; job dropped")
		return fmt.Errorf("%w: pool=%s kind=%s", ErrQueueFull, p.opts.Name, job.Kind)
	}
}

func (p *Pool) worker(ctx context.Context, id int) {
	defer p.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-p.jobs:
			if !ok {
				return
			}
			p.mu.Lock()
			if p.queued > 0 {
				p.queued--
			}
			p.inflight++
			p.mu.Unlock()
			p.metrics.Set("async_queue_depth_"+p.opts.Name, int64(p.queueLen()))

			p.execute(ctx, job)

			p.mu.Lock()
			p.inflight--
			p.mu.Unlock()
		}
	}
}

// execute runs a job with bounded retries. Terminal failures go to dead letter.
func (p *Pool) execute(ctx context.Context, job Job) {
	var lastErr error
	for attempt := 1; attempt <= p.opts.MaxAttempts; attempt++ {
		if ctx.Err() != nil {
			return
		}
		attemptCtx, cancel := context.WithTimeout(ctx, p.opts.JobTimeout)
		lastErr = job.Run(attemptCtx)
		cancel()
		if lastErr == nil {
			p.metrics.Inc("async_processed_total")
			return
		}
		p.metrics.Inc("async_retry_total")
		p.logger.Warn().Err(lastErr).
			Str("pool", p.opts.Name).
			Str("kind", job.Kind).
			Str("key", job.Key).
			Int("attempt", attempt).
			Msg("async job attempt failed")

		if attempt < p.opts.MaxAttempts {
			select {
			case <-ctx.Done():
				return
			case <-time.After(p.opts.RetryDelay):
			}
		}
	}

	p.metrics.Inc("async_dead_letter_total")
	p.logger.Error().Err(lastErr).
		Str("pool", p.opts.Name).
		Str("kind", job.Kind).
		Str("key", job.Key).
		Int("attempts", p.opts.MaxAttempts).
		Msg("async job exhausted retries; routing to dead letter")
	if p.opts.OnDeadLetter != nil {
		p.opts.OnDeadLetter(ctx, Result{Job: job, Attempt: p.opts.MaxAttempts, Err: lastErr})
	}
}

// Close stops accepting work and waits for in-flight jobs to finish.
func (p *Pool) Close() {
	p.mu.Lock()
	if p.done {
		p.mu.Unlock()
		return
	}
	p.done = true
	p.mu.Unlock()
	close(p.jobs)
	p.wg.Wait()
}

// Stats is a point-in-time snapshot of pool pressure.
type Stats struct {
	Name     string
	Workers  int
	Queued   int
	Inflight int
}

func (p *Pool) Stats() Stats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return Stats{
		Name:     p.opts.Name,
		Workers:  p.opts.Workers,
		Queued:   p.queued,
		Inflight: p.inflight,
	}
}

func (p *Pool) queueLen() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.queued
}

func (p *Pool) Name() string { return p.opts.Name }
