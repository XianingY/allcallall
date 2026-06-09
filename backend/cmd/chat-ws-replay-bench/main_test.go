package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestChatWSReplayBenchProducesAuthenticatedReplayEvidence(t *testing.T) {
	var buffer bytes.Buffer
	err := run(context.Background(), benchConfig{
		Events:       60,
		Recipients:   3,
		ReplayWindow: 12,
		ReplayLimit:  5,
		Clients:      2,
		Timeout:      2 * time.Second,
	}, &buffer)
	if err != nil {
		t.Fatalf("run chat ws replay bench failed: %v", err)
	}

	var output benchOutput
	if err := json.Unmarshal(buffer.Bytes(), &output); err != nil {
		t.Fatalf("decode bench output failed: %v\n%s", err, buffer.String())
	}
	if output.UpgradeSuccess != 2 || output.UpgradeErrors != 0 || output.ClientErrors != 0 {
		t.Fatalf("unexpected websocket status: %+v", output)
	}
	if output.ExpectedPerClient != 5 || output.TotalReplayed != 10 || output.CompleteClients != 2 {
		t.Fatalf("unexpected replay counts: %+v", output)
	}
	if !output.ScopedCorrectly || !output.MonotonicIDs || !output.MonotonicSequences {
		t.Fatalf("unexpected replay invariants: %+v", output)
	}
	if output.DuplicateEvents != 0 || output.SequenceMismatch != 0 {
		t.Fatalf("unexpected duplicate/sequence counts: %+v", output)
	}
	if output.ConnectToFirstLatency.Count != 2 || output.ConnectToLastLatency.Count != 2 {
		t.Fatalf("missing latency stats: %+v", output)
	}
}
