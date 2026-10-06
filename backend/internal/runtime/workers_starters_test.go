package runtime

import (
	"reflect"
	"testing"

	"github.com/allcallall/backend/internal/events"
)

func TestEmbeddedAgentOutboxProcessorUsesAgentWorkerOrdering(t *testing.T) {
	expected := []string{
		EventAgentRunRequested,
		EventWorkflowRequested,
		EventAgentApprovedWrite,
		EventWorkflowApprovedWrite,
		EventMCPExecutionTerminal,
	}
	if got := AgentOrderedEvents(); !reflect.DeepEqual(got, expected) {
		t.Fatalf("AgentOrderedEvents()=%v want %v", got, expected)
	}

	processor := events.NewProcessor(nil)
	ConfigureEmbeddedAgentOutboxProcessorFromEnv(processor, "embedded-order-test", expected...)
	if got := processor.OrderedEvents(); !reflect.DeepEqual(got, expected) {
		t.Fatalf("embedded ordered events=%v want %v", got, expected)
	}
}
