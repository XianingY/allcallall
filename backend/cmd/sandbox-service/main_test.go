package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/sandbox"
	"github.com/allcallall/backend/internal/testutil"
)

type handlerRunner struct {
	mu      sync.Mutex
	calls   int
	execute func(context.Context, mcpplatform.ExecutionRequest) (mcpplatform.ExecutionResult, error)
}

func (r *handlerRunner) Validate(context.Context, mcpplatform.ValidationRequest) (mcpplatform.ValidationResult, error) {
	return mcpplatform.ValidationResult{}, nil
}

func (r *handlerRunner) Execute(ctx context.Context, request mcpplatform.ExecutionRequest) (mcpplatform.ExecutionResult, error) {
	r.mu.Lock()
	r.calls++
	execute := r.execute
	r.mu.Unlock()
	if execute != nil {
		return execute(ctx, request)
	}
	return mcpplatform.ExecutionResult{
		ExecutionID: request.ExecutionID,
		JobID:       request.ExecutionID,
		Output:      map[string]any{"ok": true},
	}, nil
}

func (r *handlerRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func TestExecutionRoutesReplayLookupAndConflict(t *testing.T) {
	runner := &handlerRunner{}
	handler, signer := receiptHandler(t, runner)
	request := handlerExecutionRequest()

	first := executeRequest(t, handler, signer, request)
	if first.Code != http.StatusOK {
		t.Fatalf("first execution status=%d body=%s", first.Code, first.Body.String())
	}
	second := executeRequest(t, handler, signer, request)
	if second.Code != http.StatusOK || runner.callCount() != 1 {
		t.Fatalf("replay status=%d calls=%d body=%s", second.Code, runner.callCount(), second.Body.String())
	}

	lookupRequest := httptest.NewRequest(http.MethodGet, "/internal/v1/executions/"+request.ExecutionID, nil)
	authorizeLookupRequest(t, signer, lookupRequest, request.ExecutionID)
	lookup := httptest.NewRecorder()
	handler.ServeHTTP(lookup, lookupRequest)
	if lookup.Code != http.StatusOK {
		t.Fatalf("lookup status=%d body=%s", lookup.Code, lookup.Body.String())
	}
	var receipt mcpplatform.SandboxExecutionReceipt
	if err := json.Unmarshal(lookup.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Status != models.SandboxExecutionStatusSucceeded || receipt.RequestDigest == "" {
		t.Fatalf("unexpected lookup receipt: %#v", receipt)
	}

	conflictRequest := request
	conflictRequest.Arguments = map[string]any{"query": "different"}
	conflict := executeRequest(t, handler, signer, conflictRequest)
	if conflict.Code != http.StatusConflict || runner.callCount() != 1 {
		t.Fatalf("conflict status=%d calls=%d body=%s", conflict.Code, runner.callCount(), conflict.Body.String())
	}

	missing := httptest.NewRecorder()
	missingRequest := httptest.NewRequest(http.MethodGet, "/internal/v1/executions/missing", nil)
	authorizeLookupRequest(t, signer, missingRequest, "missing")
	handler.ServeHTTP(missing, missingRequest)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing lookup status=%d body=%s", missing.Code, missing.Body.String())
	}
}

func TestConcurrentExecutionRouteReturnsAcceptedForRunningReceipt(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	runner := &handlerRunner{execute: func(_ context.Context, request mcpplatform.ExecutionRequest) (mcpplatform.ExecutionResult, error) {
		once.Do(func() { close(started) })
		<-release
		return mcpplatform.ExecutionResult{ExecutionID: request.ExecutionID, JobID: request.ExecutionID, Output: map[string]any{"ok": true}}, nil
	}}
	handler, signer := receiptHandler(t, runner)
	request := handlerExecutionRequest()
	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		firstDone <- executeRequest(t, handler, signer, request)
	}()
	<-started

	concurrent := executeRequest(t, handler, signer, request)
	if concurrent.Code != http.StatusAccepted {
		t.Fatalf("running replay status=%d body=%s", concurrent.Code, concurrent.Body.String())
	}
	close(release)
	if first := <-firstDone; first.Code != http.StatusOK {
		t.Fatalf("winner status=%d body=%s", first.Code, first.Body.String())
	}
	if runner.callCount() != 1 {
		t.Fatalf("Runner calls=%d, want 1", runner.callCount())
	}
}

func TestSandboxCapabilityMiddlewareRejectsMissingCrossOperationAndTamperedBody(t *testing.T) {
	handler, signer := receiptHandler(t, &handlerRunner{})
	original := handlerExecutionRequest()
	originalBody, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/internal/v1/executions", bytes.NewReader(originalBody)))
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("missing capability status=%d body=%s", missing.Code, missing.Body.String())
	}

	digest, err := mcpplatform.ExecutionAuthorizationRequestDigest(original)
	if err != nil {
		t.Fatal(err)
	}
	tampered := original
	tampered.Arguments = map[string]any{"query": "tampered"}
	tamperedBody, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	tamperedRequest := httptest.NewRequest(http.MethodPost, "/internal/v1/executions", bytes.NewReader(tamperedBody))
	authorizeRequest(t, signer, tamperedRequest, digest)
	tamperedResponse := httptest.NewRecorder()
	handler.ServeHTTP(tamperedResponse, tamperedRequest)
	if tamperedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("tampered request status=%d body=%s", tamperedResponse.Code, tamperedResponse.Body.String())
	}

	replacedToken := original
	replacedToken.SecretWrapToken = "replacement-one-time-token"
	replacedTokenBody, err := json.Marshal(replacedToken)
	if err != nil {
		t.Fatal(err)
	}
	replacedTokenRequest := httptest.NewRequest(http.MethodPost, "/internal/v1/executions", bytes.NewReader(replacedTokenBody))
	authorizeRequest(t, signer, replacedTokenRequest, digest)
	replacedTokenResponse := httptest.NewRecorder()
	handler.ServeHTTP(replacedTokenResponse, replacedTokenRequest)
	if replacedTokenResponse.Code != http.StatusUnauthorized {
		t.Fatalf("replaced secret token status=%d body=%s", replacedTokenResponse.Code, replacedTokenResponse.Body.String())
	}

	validationDigest, err := mcpplatform.ValidationAuthorizationRequestDigest(mcpplatform.ValidationRequest{
		InstallationID: original.InstallationID,
		RevisionID:     original.RevisionID,
		SourceType:     original.SourceType,
		Definition:     original.Definition,
	})
	if err != nil {
		t.Fatal(err)
	}
	crossOperation := httptest.NewRequest(http.MethodPost, "/internal/v1/executions", bytes.NewReader(originalBody))
	token, err := signer.Issue(http.MethodPost, "/internal/v1/installations/validate", validationDigest)
	if err != nil {
		t.Fatal(err)
	}
	crossOperation.Header.Set("Authorization", "Bearer "+token)
	crossResponse := httptest.NewRecorder()
	handler.ServeHTTP(crossResponse, crossOperation)
	if crossResponse.Code != http.StatusUnauthorized {
		t.Fatalf("cross-operation request status=%d body=%s", crossResponse.Code, crossResponse.Body.String())
	}

	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status=%d body=%s", health.Code, health.Body.String())
	}
}

func receiptHandler(t *testing.T, runner *handlerRunner) (http.Handler, *mcpplatform.SandboxCapabilitySigner) {
	t.Helper()
	db := testutil.OpenSQLite(t, "sandbox-handler.db")
	if err := db.AutoMigrate(&models.SandboxExecutionReceipt{}); err != nil {
		t.Fatal(err)
	}
	service := sandbox.NewService(runner, nil).WithReceiptStore(sandbox.NewReceiptStore(db))
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := mcpplatform.NewSandboxCapabilitySigner(privateKey, 0)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := mcpplatform.NewSandboxCapabilityVerifier(privateKey.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	return newHandler(service, verifier), signer
}

func executeRequest(t *testing.T, handler http.Handler, signer *mcpplatform.SandboxCapabilitySigner, request mcpplatform.ExecutionRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := mcpplatform.ExecutionAuthorizationRequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest := httptest.NewRequest(http.MethodPost, "/internal/v1/executions", bytes.NewReader(body))
	authorizeRequest(t, signer, httpRequest, digest)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httpRequest)
	return recorder
}

func authorizeLookupRequest(t *testing.T, signer *mcpplatform.SandboxCapabilitySigner, request *http.Request, executionID string) {
	t.Helper()
	digest, err := mcpplatform.SandboxLookupRequestDigest(executionID)
	if err != nil {
		t.Fatal(err)
	}
	authorizeRequest(t, signer, request, digest)
}

func authorizeRequest(t *testing.T, signer *mcpplatform.SandboxCapabilitySigner, request *http.Request, digest string) {
	t.Helper()
	token, err := signer.Issue(request.Method, request.URL.EscapedPath(), digest)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
}

func handlerExecutionRequest() mcpplatform.ExecutionRequest {
	return mcpplatform.ExecutionRequest{
		ExecutionID:    "mcp:handler-test",
		OrganizationID: 1,
		UserID:         7,
		ConversationID: 42,
		RunID:          99,
		RunRef:         "agent:99",
		ToolCallID:     "call-1",
		InstallationID: 11,
		RevisionID:     12,
		ToolID:         13,
		SourceType:     models.MCPInstallationSourceOCI,
		Definition: mcpplatform.InstallationDefinition{
			Transport: "stdio",
			ImageRef:  "registry.example.com/tool@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		ToolName:        "search",
		Arguments:       map[string]any{"query": "security", "large_id": int64(9_007_199_254_740_993)},
		SecretWrapToken: "one-time-token",
		TimeoutMS:       30_000,
		OutputLimit:     mcpplatform.DefaultOutputLimit,
	}
}
