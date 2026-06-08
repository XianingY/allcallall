package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/allcallall/backend/internal/models"
)

func TestInterviewBenchProducesAgentPipelineEvidence(t *testing.T) {
	var buffer bytes.Buffer
	err := run(context.Background(), benchConfig{
		Conversations: 3,
		BatchSize:     10,
		Provider:      models.AgentRunSourceRules,
	}, &buffer)
	if err != nil {
		t.Fatalf("run interview bench failed: %v", err)
	}

	var output benchOutput
	if err := json.Unmarshal(buffer.Bytes(), &output); err != nil {
		t.Fatalf("decode bench output failed: %v\n%s", err, buffer.String())
	}
	if output.QueuedRuns != 3 || output.ReadyRuns != 3 || output.FailedRuns != 0 {
		t.Fatalf("unexpected run counts: %+v", output)
	}
	if output.ProcessedEvents != 9 || output.PendingOutboxEvents != 0 || output.FailedOutboxEvents != 0 {
		t.Fatalf("unexpected outbox counts: %+v", output)
	}
	if output.AgentSteps != 6 || output.AgentToolCalls != 18 {
		t.Fatalf("unexpected agent execution counts: steps=%d tool_calls=%d", output.AgentSteps, output.AgentToolCalls)
	}
	if output.SystemMessages != 3 || output.FollowUpTasks != 3 || output.AgentMemories != 3 {
		t.Fatalf("unexpected tool side effects: messages=%d tasks=%d memories=%d", output.SystemMessages, output.FollowUpTasks, output.AgentMemories)
	}
	if output.ExecuteRunLatency.Count != 3 || output.QueueLatency.Count != 3 {
		t.Fatalf("missing latency stats: %+v", output)
	}
	if output.Counters["agent_run_queued_total"] != 3 || output.Counters["outbox_publish_total"] != 9 {
		t.Fatalf("unexpected counters: %+v", output.Counters)
	}
}
