package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type fakeReadToolExecutor struct {
	called string
	input  string
}

func (f *fakeReadToolExecutor) ExecuteReadOnlyTool(_ context.Context, _ uint64, _ uint64, toolName, inputJSON string) (string, error) {
	f.called = toolName
	f.input = inputJSON
	return `{"member_count":2,"peer_user_ids":[8]}`, nil
}

func TestMCPInitializeAndListTools(t *testing.T) {
	server := mcpServer{organizationID: 1, userID: 7, executor: &fakeReadToolExecutor{}}
	var out bytes.Buffer
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
	}, "\n") + "\n"
	if err := server.serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("serve: %v", err)
	}
	responses := decodeResponses(t, out.String())
	if len(responses) != 2 {
		t.Fatalf("expected 2 responses, got %d: %s", len(responses), out.String())
	}
	tools := responses[1]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 4 {
		t.Fatalf("expected 4 read-only tools, got %d", len(tools))
	}
	for _, raw := range tools {
		tool := raw.(map[string]any)
		if tool["name"] == "write_conversation_message" {
			t.Fatalf("side-effect tool should not be exposed: %+v", tool)
		}
	}
}

func TestMCPToolCallUsesReadOnlyExecutor(t *testing.T) {
	executor := &fakeReadToolExecutor{}
	server := mcpServer{organizationID: 1, userID: 7, executor: executor}
	var out bytes.Buffer
	input := `{"jsonrpc":"2.0","id":"call-1","method":"tools/call","params":{"name":"query_conversation_members","arguments":{"conversation_id":42}}}` + "\n"
	if err := server.serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("serve: %v", err)
	}
	if executor.called != "query_conversation_members" || !strings.Contains(executor.input, `"conversation_id":42`) {
		t.Fatalf("executor not called correctly: tool=%s input=%s", executor.called, executor.input)
	}
	responses := decodeResponses(t, out.String())
	result := responses[0]["result"].(map[string]any)
	if result["isError"] != false {
		t.Fatalf("expected successful tool call: %+v", result)
	}
}

func TestMCPToolCallRejectsSideEffectTool(t *testing.T) {
	server := mcpServer{organizationID: 1, userID: 7, executor: &fakeReadToolExecutor{}}
	var out bytes.Buffer
	input := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"write_conversation_message","arguments":{"conversation_id":42}}}` + "\n"
	if err := server.serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("serve: %v", err)
	}
	responses := decodeResponses(t, out.String())
	if responses[0]["error"] == nil {
		t.Fatalf("expected error response: %s", out.String())
	}
}

func decodeResponses(t *testing.T, raw string) []map[string]any {
	t.Helper()
	scanner := bufio.NewScanner(strings.NewReader(raw))
	var responses []map[string]any
	for scanner.Scan() {
		var item map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			t.Fatalf("decode response %q: %v", scanner.Text(), err)
		}
		responses = append(responses, item)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan responses: %v", err)
	}
	return responses
}
