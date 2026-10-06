package agent

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/models"
)

func pythonRuntimeDir() string {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		return ""
	}
	candidates := []string{
		filepath.Join(repoRoot, "..", "allcallall-agent-runtime-performance"),
		filepath.Join(repoRoot, "..", "allcallall-agent-runtime"),
	}
	for _, candidate := range candidates {
		python := filepath.Join(candidate, ".venv", "bin", "python")
		if info, err := os.Stat(python); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func TestPythonLangGraphRuntimeHTTPContract(t *testing.T) {
	runtimeDir := pythonRuntimeDir()
	if runtimeDir == "" {
		t.Skip("Python agent runtime checkout/.venv not found; cross-repo HTTP contract test skipped")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release reserved port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	sqlitePath := filepath.Join(t.TempDir(), "contract.sqlite")
	python := filepath.Join(runtimeDir, ".venv", "bin", "python")
	cmd := exec.Command(
		python,
		"-m",
		"uvicorn",
		"allcallall_agent_runtime.main:app",
		"--host",
		"127.0.0.1",
		"--port",
		strconv.Itoa(port),
	)
	cmd.Dir = filepath.Join(runtimeDir, "services", "agent-runtime")
	cmd.Env = append(
		os.Environ(),
		"PY_AGENT_PROVIDER=rules",
		"PY_AGENT_PROVIDER_STRICT=true",
		"PY_AGENT_API_TOKEN=",
		"PY_AGENT_DEPLOYMENT_MODE=multi_replica",
		"PY_AGENT_ENABLE_TOOL_QUEUE=false",
		"PY_AGENT_ENABLE_BADCASE_CAPTURE=false",
		"PY_AGENT_CHECKPOINT_STORE=sqlite",
		fmt.Sprintf("PY_AGENT_CHECKPOINT_SQLITE_PATH=%s", sqlitePath),
	)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start Python runtime: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
		if t.Failed() {
			t.Logf("Python runtime output:\n%s", output.String())
		}
	})

	healthClient := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, healthErr := healthClient.Get(baseURL + "/health")
		if healthErr == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("Python runtime did not become ready:\n%s", output.String())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	runtime := NewPythonLangGraphRuntime(baseURL, &http.Client{Timeout: 30 * time.Second})
	input := WorkflowRuntimeRequest{
		RequestID:          "integration-request",
		ExecutionID:        "workflow:987654",
		ExpectedCheckpoint: 0,
		OrganizationID:     1,
		UserID:             7,
		ConversationID:     42,
		WorkflowRunID:      987654,
		Preset:             WorkflowPresetMeetingBrief,
		Goal:               "请生成会议复盘，关注风险和行动项。",
		MeetingTranscripts: []WorkflowRuntimeTranscript{
			{
				ID:                 10,
				RecordingSessionID: 20,
				RecordingFileID:    30,
				StartMS:            1000,
				EndMS:              5000,
				Text:               "本次会议确认需要跟进安全审批，预算截止日期存在风险。",
				Speaker:            "speaker",
			},
		},
		ContextChunks: []WorkflowRuntimeContextChunk{
			{
				ChunkID:             "10",
				SourceType:          "meeting_transcript",
				SourceID:            "10",
				SourceTitle:         "Task Eval Meeting",
				Title:               "Task Eval Meeting",
				Snippet:             "本次会议确认需要跟进安全审批，预算截止日期存在风险。",
				Score:               10,
				RetrievalMode:       "rules",
				RecordingSessionID:  uint64Ptr(20),
				RecordingFileID:     uint64Ptr(30),
				TranscriptSegmentID: uint64Ptr(10),
				StartMS:             int64Ptr(1000),
				EndMS:               int64Ptr(5000),
			},
		},
		ToolPolicy: WorkflowRuntimeToolPolicy{
			ReadTools: []string{ToolQueryContextChunks},
			WriteTools: []string{
				ToolWriteConversationMessage,
				ToolCreateFollowUpTask,
				ToolUpsertConversationMemory,
			},
		},
		MaxIterations: map[string]int{"searcher": 3, "risk_analyst": 2},
		ContextManifest: &ContextManifest{
			Selected:        map[string]int{"meeting_transcripts": 1, "context_chunks": 1},
			Truncated:       []string{},
			SerializedBytes: 256,
			EstimatedTokens: 64,
			SQLStatements:   5,
		},
	}

	initial, err := runtime.RunWorkflow(ctx, input)
	if err != nil {
		t.Fatalf("run Python workflow over HTTP: %v", err)
	}
	if err := validateInitialWorkflowRuntimeResponse(models.WorkflowRun{}, input.ExecutionID, initial); err != nil {
		t.Fatalf("initial Python response contract: %v", err)
	}
	if initial.PendingApproval == nil || len(initial.PendingApproval.Tools) == 0 {
		t.Fatalf("expected pending approval, got response: %+v", initial)
	}

	decisions := make([]WorkflowRuntimeDecision, 0, len(initial.PendingApproval.Tools))
	for _, tool := range initial.PendingApproval.Tools {
		decisions = append(decisions, WorkflowRuntimeDecision{
			ToolCallID: tool.ToolCallID,
			Decision:   "approve",
		})
	}
	resume := WorkflowRuntimeResumeRequest{
		RequestID:                 input.RequestID,
		ExecutionID:               input.ExecutionID,
		ExpectedCheckpointVersion: initial.CheckpointVersion,
		OrganizationID:            input.OrganizationID,
		UserID:                    input.UserID,
		ConversationID:            input.ConversationID,
		WorkflowRunID:             input.WorkflowRunID,
		Resume: WorkflowRuntimeResume{
			ApprovalRequestID: initial.PendingApproval.ApprovalRequestID,
			Decisions:         decisions,
		},
	}
	resumed, err := runtime.ResumeWorkflow(ctx, WorkflowPresetMeetingBrief, resume)
	if err != nil {
		t.Fatalf("resume Python workflow over HTTP: %v", err)
	}
	if err := validateResumedWorkflowRuntimeResponse(
		initial.CheckpointVersion,
		input.ExecutionID,
		decisions,
		resumed,
	); err != nil {
		t.Fatalf("resumed Python response contract: %v", err)
	}
}

func uint64Ptr(value uint64) *uint64 { return &value }

func int64Ptr(value int64) *int64 { return &value }
