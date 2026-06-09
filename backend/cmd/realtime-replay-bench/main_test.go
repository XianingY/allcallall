package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestRealtimeReplayBenchProducesScopedReplayEvidence(t *testing.T) {
	var buffer bytes.Buffer
	err := run(context.Background(), benchConfig{
		Events:       30,
		Recipients:   3,
		ReplayWindow: 8,
		ReplayLimit:  5,
	}, &buffer)
	if err != nil {
		t.Fatalf("run realtime replay bench failed: %v", err)
	}

	var output benchOutput
	if err := json.Unmarshal(buffer.Bytes(), &output); err != nil {
		t.Fatalf("decode bench output failed: %v\n%s", err, buffer.String())
	}
	if output.TotalEventsWritten != 30 || output.TargetEvents != 10 {
		t.Fatalf("unexpected written event counts: %+v", output)
	}
	if output.ReplayedEvents != 5 || output.ExpectedReplayed != 5 {
		t.Fatalf("unexpected replay count: %+v", output)
	}
	if !output.ScopedCorrectly || !output.MonotonicIDs || !output.MonotonicSequences || output.SequenceMismatch != 0 {
		t.Fatalf("unexpected replay invariants: %+v", output)
	}
	if output.WriteLatency.Count != 30 || output.ReplayLatency.Count != 1 {
		t.Fatalf("unexpected latency stats: %+v", output)
	}
}
