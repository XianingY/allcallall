package settlement

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"time"

	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/mq"
)

const (
	// idlePollInterval 队列空闲时的轮询间隔。没有消息属于正常状态，
	// 不计入失败，保持短轮询即可。
	idlePollInterval = 100 * time.Millisecond
	// fetchBaseBackoff / fetchMaxBackoff 是 Fetch 连续失败时的指数退避区间。
	// 没有退避时，broker/网络抖动会让 worker 以零间隔反复 Fetch，
	// CPU 打满并刷爆日志。
	fetchBaseBackoff = 100 * time.Millisecond
	fetchMaxBackoff  = 5 * time.Second
	// applyBaseBackoff / applyMaxBackoff 是业务处理失败后的退避区间。
	applyBaseBackoff = 200 * time.Millisecond
	applyMaxBackoff  = 3 * time.Second
	// commitRetryBackoff 是提交失败后的等待时长。
	commitRetryBackoff = 500 * time.Millisecond
	// maxBackoffShift 限制位移量，1<<30 已远超任何有意义的等待时长。
	maxBackoffShift = 30
)

type Worker struct {
	consumer mq.Consumer
	service  *Service
	logger   zerolog.Logger
}

func NewWorker(consumer mq.Consumer, service *Service, logger zerolog.Logger) *Worker {
	return &Worker{
		consumer: consumer,
		service:  service,
		logger:   logger.With().Str("component", "settlement_worker").Logger(),
	}
}

func (w *Worker) Run(ctx context.Context) error {
	if w == nil || w.consumer == nil || w.service == nil {
		return errors.New("settlement worker is not initialized")
	}

	fetchFailures := 0
	applyFailures := 0

	for {
		message, err := w.consumer.Fetch(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			if errors.Is(err, mq.ErrNoMessages) {
				fetchFailures = 0
				if !waitFor(ctx, idlePollInterval) {
					return ctx.Err()
				}
				continue
			}
			fetchFailures++
			delay := exponentialBackoff(fetchFailures, fetchBaseBackoff, fetchMaxBackoff)
			w.logger.Warn().Err(err).
				Int("consecutive_failures", fetchFailures).
				Dur("backoff", delay).
				Msg("settlement fetch failed; backing off")
			if !waitFor(ctx, delay) {
				return ctx.Err()
			}
			continue
		}
		fetchFailures = 0

		event, err := DecodeRoomEndedMessage(message)
		if err != nil {
			w.logger.Warn().Err(err).Msg("invalid settlement event")
			_ = w.consumer.Commit(ctx, message)
			continue
		}

		record, err := w.service.ApplyRoomEnded(ctx, event)
		if err != nil {
			applyFailures++
			delay := exponentialBackoff(applyFailures, applyBaseBackoff, applyMaxBackoff)
			// 这里刻意不提交、也不跳过消息：结算涉及资金，静默丢弃会造成漏结算。
			// 代价是持续失败的事件会被反复重试（poison message），
			// 根治方式是接入 internal/async 的死信队列，把达到重试上限的事件
			// 转投 DLQ 并告警，而不是无限重试。
			w.logger.Error().Err(err).
				Str("event_id", event.EventID).
				Int("consecutive_failures", applyFailures).
				Dur("backoff", delay).
				Msg("settlement apply failed; retrying after backoff")
			if !waitFor(ctx, delay) {
				return ctx.Err()
			}
			continue
		}
		applyFailures = 0

		if err := w.consumer.Commit(ctx, message); err != nil {
			w.logger.Warn().Err(err).
				Str("event_id", event.EventID).
				Dur("backoff", commitRetryBackoff).
				Msg("settlement commit failed; retrying after backoff")
			if !waitFor(ctx, commitRetryBackoff) {
				return ctx.Err()
			}
			continue
		}
		w.logger.Info().
			Str("event_id", event.EventID).
			Uint64("settlement_id", record.ID).
			Uint64("room_id", event.RoomID).
			Uint64("user_id", event.UserID).
			Msg("settlement event applied")
	}
}

// waitFor 等待 d；ctx 先结束时返回 false。
// 使用 Timer 而非 time.After，避免循环里每次都遗留一个已触发的定时器。
func waitFor(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// exponentialBackoff 返回第 failures 次（从 1 起算）连续失败对应的等待时长：
// base * 2^(failures-1)，上限 max，并施加 ±20% 抖动，避免多个 worker 在依赖
// 恢复的同一瞬间同时冲击上游。过早触顶的位移会溢出为负数，这里统一截到 max。
func exponentialBackoff(failures int, base, max time.Duration) time.Duration {
	if failures < 1 {
		failures = 1
	}
	shift := failures - 1
	if shift > maxBackoffShift {
		shift = maxBackoffShift
	}
	delay := base << shift
	if delay <= 0 || delay > max {
		delay = max
	}
	jittered := jitterDuration(delay)
	if jittered > max {
		jittered = max
	}
	return jittered
}

func jitterDuration(delay time.Duration) time.Duration {
	// 800..1199 千分比对应原有 ±20% 抖动；失败时退回无抖动延迟，
	// 避免把随机源故障升级为结算 worker 故障。
	jitter, err := rand.Int(rand.Reader, big.NewInt(400))
	if err != nil {
		return delay
	}
	return delay * time.Duration(800+jitter.Int64()) / 1000
}
