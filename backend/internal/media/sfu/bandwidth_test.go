package sfu

import (
	"testing"
)

func TestBandwidthManagerOptimisticWithoutEstimate(t *testing.T) {
	m := NewBandwidthManager()
	// No downlink estimate and no track bitrate -> always forward (optimistic).
	if !m.ShouldForward("sub1", "track1") {
		t.Fatal("expected forward when nothing is known")
	}
}

func TestBandwidthManagerAudioFloorAlwaysForwarded(t *testing.T) {
	m := NewBandwidthManager()
	m.SetDownlink("sub1", 100_000) // very weak link
	// A track at/below the minimum forward bitrate must always be allowed even
	// though the subscriber link is tiny.
	m.RegisterTrack("audio", DefaultMinForwardBitrate)
	if !m.ShouldForward("sub1", "audio") {
		t.Fatal("audio floor track should always be forwarded")
	}
}

func TestBandwidthManagerBudgetEnforcement(t *testing.T) {
	m := NewBandwidthManager()
	const downlink = 1_000_000 // 1 Mbps
	m.SetDownlink("sub1", downlink)
	// 15% headroom -> ~850 kbps available.
	m.RegisterTrack("vid1", 500_000)
	m.RegisterTrack("vid2", 500_000)

	if !m.ShouldForward("sub1", "vid1") {
		t.Fatal("first video track should fit the budget")
	}
	m.MarkForwarded("sub1", "vid1")

	if m.ShouldForward("sub1", "vid2") {
		t.Fatal("second video track should exceed the budget and be denied")
	}
}

func TestBandwidthManagerForwardBudget(t *testing.T) {
	m := NewBandwidthManager()
	m.SetDownlink("sub1", 1_000_000)
	m.RegisterTrack("vid1", 300_000)
	m.MarkForwarded("sub1", "vid1")

	budget := m.ForwardBudget("sub1")
	// ~850k available minus 300k used = ~550k remaining.
	if budget <= 0 || budget > 600_000 {
		t.Fatalf("unexpected forward budget: %d", budget)
	}

	// Unknown subscriber has no budget.
	if m.ForwardBudget("unknown") != 0 {
		t.Fatal("unknown subscriber should report zero budget")
	}
}

func TestBandwidthManagerMarkUnmarkForwarded(t *testing.T) {
	m := NewBandwidthManager()
	m.MarkForwarded("sub1", "vid1")
	m.MarkForwarded("sub1", "vid2")
	if m.ForwardBudget("sub1") == 0 {
		// budget is 0 only because no downlink estimate; just ensure no panic
		// and that unmarking works.
	}
	m.UnmarkForwarded("sub1", "vid1")
	m.UnmarkForwarded("sub1", "vid2")
	m.UnmarkForwarded("sub1", "vid1") // idempotent

	stats := m.Stats()
	if stats.Forwarded != 0 {
		t.Fatalf("expected no forwarded tracks after unmarking, got %d", stats.Forwarded)
	}
}

func TestBandwidthManagerThrottledCounter(t *testing.T) {
	m := NewBandwidthManager()
	m.RecordThrottled()
	m.RecordThrottled()
	if got := m.Stats().Throttled; got != 2 {
		t.Fatalf("expected 2 throttled decisions, got %d", got)
	}
}

func TestBandwidthManagerDownlinkHistory(t *testing.T) {
	m := NewBandwidthManager()
	for i := 0; i < 15; i++ {
		m.SetDownlink("sub1", 100_000+i*1000)
	}
	stats := m.Stats()
	if stats.DownlinkBps["sub1"] != 100_000+14*1000 {
		t.Fatalf("latest downlink not retained: %d", stats.DownlinkBps["sub1"])
	}
	if len(stats.DownlinkBps) != 1 {
		t.Fatalf("expected one participant, got %d", len(stats.DownlinkBps))
	}
}

func TestBandwidthManagerForgetTrack(t *testing.T) {
	m := NewBandwidthManager()
	m.RegisterTrack("vid1", 500_000)
	m.MarkForwarded("sub1", "vid1")
	m.ForgetTrack("vid1")

	if _, ok := m.trackBitrate["vid1"]; ok {
		t.Fatal("track bitrate should be forgotten")
	}
	if _, ok := m.forwarded["sub1"]["vid1"]; ok {
		t.Fatal("forwarded entry should be cleaned up")
	}
}

func TestBandwidthManagerForgetParticipant(t *testing.T) {
	m := NewBandwidthManager()
	m.SetDownlink("sub1", 1_000_000)
	m.RegisterTrack("vid1", 500_000)
	m.MarkForwarded("sub1", "vid1")

	m.ForgetParticipant("sub1")

	if m.GetDownlink("sub1") != 0 {
		t.Fatal("downlink should be cleared")
	}
	if _, ok := m.forwarded["sub1"]; ok {
		t.Fatal("forwarded set should be cleared")
	}
}

func TestBandwidthManagerStatsEnabled(t *testing.T) {
	m := NewBandwidthManager()
	m.RegisterTrack("vid1", 123)
	stats := m.Stats()
	if !stats.Enabled {
		t.Fatal("manager stats should report enabled")
	}
	if stats.Tracks != 1 {
		t.Fatalf("expected 1 tracked track, got %d", stats.Tracks)
	}
}

func TestBandwidthControllerFactoryBindsPending(t *testing.T) {
	c := NewBandwidthController()
	c.SetPending("p1")
	f := c.Factory()
	est, err := f()
	if err != nil {
		t.Fatalf("factory returned error: %v", err)
	}
	if est == nil {
		t.Fatal("factory returned nil estimator")
	}
	// Calling OnTargetBitrateChange must not panic (controller wiring uses it).
	called := false
	est.OnTargetBitrateChange(func(int) { called = true })
	est.OnTargetBitrateChange(func(int) {})
	_ = called

	// After consumption the pending binding is cleared.
	c.SetPending("p2")
	est2, err := c.Factory()()
	if err != nil {
		t.Fatalf("second factory call error: %v", err)
	}
	if est2 == nil {
		t.Fatal("second estimator nil")
	}
}

func TestBandwidthControllerNilSafe(t *testing.T) {
	// Manager() on a typed-nil is handled in callers, but ensure no panic.
	var c *BandwidthController
	if c.Manager() != nil {
		t.Fatal("nil controller manager should be nil")
	}
}
