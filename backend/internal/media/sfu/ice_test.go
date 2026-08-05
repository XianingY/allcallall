package sfu

import (
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func candidate(value string) webrtc.ICECandidateInit {
	return webrtc.ICECandidateInit{Candidate: value}
}

func TestCandidateBufferBuffersUntilRemoteReady(t *testing.T) {
	buffer := NewCandidateBuffer(0)

	if buffer.RemoteReady() {
		t.Fatal("expected buffer to start without a remote description")
	}
	if buffer.Accept(candidate("a")) {
		t.Fatal("expected early candidate to be buffered, not applied")
	}
	if buffer.Accept(candidate("b")) {
		t.Fatal("expected early candidate to be buffered, not applied")
	}
	if got := buffer.Pending(); got != 2 {
		t.Fatalf("expected 2 pending candidates, got %d", got)
	}

	flushed := buffer.MarkRemoteReady()
	if len(flushed) != 2 {
		t.Fatalf("expected 2 flushed candidates, got %d", len(flushed))
	}
	if flushed[0].Candidate != "a" || flushed[1].Candidate != "b" {
		t.Fatalf("expected arrival order preserved, got %q then %q", flushed[0].Candidate, flushed[1].Candidate)
	}
	if got := buffer.Pending(); got != 0 {
		t.Fatalf("expected buffer drained after flush, got %d", got)
	}

	if !buffer.Accept(candidate("c")) {
		t.Fatal("expected candidate to be applied directly once remote is ready")
	}
	if extra := buffer.MarkRemoteReady(); extra != nil {
		t.Fatalf("expected second flush to be empty, got %d entries", len(extra))
	}
}

func TestCandidateBufferDropsOldestWhenFull(t *testing.T) {
	buffer := NewCandidateBuffer(2)

	buffer.Accept(candidate("a"))
	buffer.Accept(candidate("b"))
	buffer.Accept(candidate("c"))

	if got := buffer.Pending(); got != 2 {
		t.Fatalf("expected buffer to stay capped at 2, got %d", got)
	}
	if got := buffer.Dropped(); got != 1 {
		t.Fatalf("expected 1 dropped candidate, got %d", got)
	}

	flushed := buffer.MarkRemoteReady()
	if len(flushed) != 2 || flushed[0].Candidate != "b" || flushed[1].Candidate != "c" {
		t.Fatalf("expected newest candidates retained, got %+v", flushed)
	}
}

func TestGatherPolicyTimeout(t *testing.T) {
	policy := GatherPolicy{TrickleTimeout: 50 * time.Millisecond, BlockingTimeout: 2 * time.Second}

	if got := policy.Timeout(true); got != 50*time.Millisecond {
		t.Fatalf("expected trickle timeout, got %s", got)
	}
	if got := policy.Timeout(false); got != 2*time.Second {
		t.Fatalf("expected blocking timeout, got %s", got)
	}

	defaults := DefaultGatherPolicy()
	empty := GatherPolicy{}
	if got := empty.Timeout(true); got != defaults.TrickleTimeout {
		t.Fatalf("expected default trickle timeout, got %s", got)
	}
	if got := empty.Timeout(false); got != defaults.BlockingTimeout {
		t.Fatalf("expected default blocking timeout, got %s", got)
	}
}

func TestWaitGather(t *testing.T) {
	done := make(chan struct{})
	close(done)
	if !WaitGather(done, 10*time.Millisecond) {
		t.Fatal("expected completed gathering to be reported")
	}

	pending := make(chan struct{})
	start := time.Now()
	if WaitGather(pending, 20*time.Millisecond) {
		t.Fatal("expected timeout to be reported as incomplete")
	}
	if elapsed := time.Since(start); elapsed < 15*time.Millisecond {
		t.Fatalf("expected wait to respect the timeout, returned after %s", elapsed)
	}

	if WaitGather(nil, time.Second) {
		t.Fatal("expected nil channel to report incomplete")
	}
	if WaitGather(pending, 0) {
		t.Fatal("expected zero timeout to poll without blocking")
	}
}
