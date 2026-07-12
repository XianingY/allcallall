package mcpplatform

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPSandboxClientLookupExecution(t *testing.T) {
	want := SandboxExecutionReceipt{
		ExecutionID:    "mcp:receipt-1",
		RequestDigest:  "sha256:receipt-1",
		Status:         SandboxExecutionStatusSucceeded,
		JobID:          "job-1",
		OrganizationID: 1,
		UserID:         7,
		ConversationID: 42,
		RunID:          99,
		RunRef:         "agent:99",
		ToolCallID:     "call-1",
		InstallationID: 10,
		RevisionID:     11,
		ToolID:         12,
		ToolName:       "search",
		Output:         map[string]any{"ok": true},
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/internal/v1/executions/mcp:receipt-1" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(want)
	}))
	defer server.Close()
	client, err := NewHTTPSandboxClient(server.URL, 0, testSandboxCapabilitySigner(t))
	if err != nil {
		t.Fatal(err)
	}

	got, err := client.LookupExecution(context.Background(), want.ExecutionID)
	if err != nil {
		t.Fatalf("lookup receipt: %v", err)
	}
	if got.ExecutionID != want.ExecutionID || got.RequestDigest != want.RequestDigest || got.Status != want.Status || got.ToolID != want.ToolID {
		t.Fatalf("unexpected receipt: %+v", got)
	}
}

func TestHTTPSandboxClientClassifiesReceiptLookupStatus(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		statusCode int
		want       error
	}{
		{name: "not found", statusCode: http.StatusNotFound, want: ErrSandboxExecutionNotFound},
		{name: "digest conflict", statusCode: http.StatusConflict, want: ErrSandboxExecutionConflict},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(testCase.statusCode)
			}))
			defer server.Close()
			client, err := NewHTTPSandboxClient(server.URL, 0, testSandboxCapabilitySigner(t))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.LookupExecution(context.Background(), "mcp:missing"); !errors.Is(err, testCase.want) {
				t.Fatalf("expected %v, got %v", testCase.want, err)
			}
		})
	}
}

func TestHTTPSandboxClientBoundsReceiptEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"output":{"value":"` + strings.Repeat("x", sandboxResponseLimit) + `"}}`))
	}))
	defer server.Close()
	client, err := NewHTTPSandboxClient(server.URL, 0, testSandboxCapabilitySigner(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.LookupExecution(context.Background(), "mcp:oversized"); !errors.Is(err, ErrOutputTooLarge) {
		t.Fatalf("expected bounded receipt error, got %v", err)
	}
}

func TestHTTPSandboxClientRejectsUnsafeBaseURL(t *testing.T) {
	for _, baseURL := range []string{
		"ftp://sandbox.example.com",
		"file:///tmp/sandbox.sock",
		"https://user:password@sandbox.example.com",
		"sandbox.example.com",
	} {
		t.Run(baseURL, func(t *testing.T) {
			if _, err := NewHTTPSandboxClient(baseURL, time.Second, testSandboxCapabilitySigner(t)); err == nil {
				t.Fatal("expected unsafe sandbox base URL rejection")
			}
		})
	}
}

func TestExecutionRequestDigestExcludesOneTimeSecretToken(t *testing.T) {
	request := ExecutionRequest{
		ExecutionID: "mcp:digest", OrganizationID: 1, UserID: 7, ConversationID: 42,
		RunID: 99, RunRef: "agent:99", ToolCallID: "call-1", InstallationID: 10,
		RevisionID: 11, ToolID: 12, SourceType: "https", ToolName: "search",
		Arguments: map[string]any{"query": "security"}, SecretWrapToken: "first-one-time-token",
	}
	first, err := ExecutionRequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	request.SecretWrapToken = "fresh-one-time-token"
	second, err := ExecutionRequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("one-time secret token changed semantic digest: %q != %q", first, second)
	}
	request.Arguments["query"] = "different"
	changed, err := ExecutionRequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	if changed == first {
		t.Fatal("semantic argument change did not change execution digest")
	}
}

func TestExecutionAuthorizationRequestDigestIncludesOneTimeSecretToken(t *testing.T) {
	request := ExecutionRequest{ExecutionID: "mcp:authorization", SecretWrapToken: "first-token"}
	first, err := ExecutionAuthorizationRequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	request.SecretWrapToken = "replacement-token"
	second, err := ExecutionAuthorizationRequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("authorization digest did not bind the one-time secret token")
	}
}

func TestValidationAuthorizationRequestDigestIncludesOneTimeSecretToken(t *testing.T) {
	request := ValidationRequest{InstallationID: 10, RevisionID: 11, SecretWrapToken: "first-token"}
	first, err := ValidationAuthorizationRequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	request.SecretWrapToken = "replacement-token"
	second, err := ValidationAuthorizationRequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("validation authorization digest did not bind the one-time secret token")
	}
}

func TestExecutionRequestDigestNormalizesEmptyCollectionsAndLimits(t *testing.T) {
	request := ExecutionRequest{
		ExecutionID: " mcp:normalized ", OrganizationID: 1, UserID: 7, ConversationID: 42,
		RunID: 99, RunRef: " agent:99 ", ToolCallID: " call-1 ", InstallationID: 10,
		RevisionID: 11, ToolID: 12, SourceType: " HTTPS ", ToolName: " search ",
	}
	implicit, err := ExecutionRequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	request.ExecutionID = "mcp:normalized"
	request.RunRef = "agent:99"
	request.ToolCallID = "call-1"
	request.SourceType = "https"
	request.ToolName = "search"
	request.TimeoutMS = DefaultExecutionTimeout.Milliseconds()
	request.OutputLimit = DefaultOutputLimit
	request.Arguments = map[string]any{}
	request.Definition.Command = []string{}
	request.Definition.Args = []string{}
	request.Definition.Config = map[string]any{}
	request.Definition.NetworkAllowlist = []string{}
	explicit, err := ExecutionRequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	if implicit != explicit {
		t.Fatalf("normalized digest drifted: %q != %q", implicit, explicit)
	}
}

func TestHTTPSandboxClientSignsValidateExecuteAndLookup(t *testing.T) {
	_, signer, verifier := sandboxCapabilityTestPair(t)
	tokens := make(map[string]struct{})
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		token, err := SandboxAuthorizationToken(request)
		if err != nil {
			t.Errorf("missing capability for %s %s", request.Method, request.URL.Path)
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		if _, exists := tokens[token]; exists {
			t.Errorf("capability token was reused for %s %s", request.Method, request.URL.Path)
		}
		tokens[token] = struct{}{}

		var digest string
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/internal/v1/installations/validate":
			var input ValidationRequest
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Error(err)
				response.WriteHeader(http.StatusBadRequest)
				return
			}
			digest, err = ValidationAuthorizationRequestDigest(input)
			_ = json.NewEncoder(response).Encode(ValidationResult{ScanStatus: "clean", Tools: []DiscoveredTool{}})
		case request.Method == http.MethodPost && request.URL.Path == "/internal/v1/executions":
			var input ExecutionRequest
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Error(err)
				response.WriteHeader(http.StatusBadRequest)
				return
			}
			digest, err = ExecutionAuthorizationRequestDigest(input)
			_ = json.NewEncoder(response).Encode(SandboxExecutionReceipt{ExecutionID: input.ExecutionID, Status: SandboxExecutionStatusSucceeded})
		case request.Method == http.MethodGet && request.URL.Path == "/internal/v1/executions/mcp:signed":
			digest, err = SandboxLookupRequestDigest("mcp:signed")
			_ = json.NewEncoder(response).Encode(SandboxExecutionReceipt{ExecutionID: "mcp:signed", Status: SandboxExecutionStatusSucceeded})
		default:
			response.WriteHeader(http.StatusNotFound)
			return
		}
		if err != nil {
			t.Error(err)
			return
		}
		if err := verifier.Verify(token, request.Method, request.URL.EscapedPath(), digest); err != nil {
			t.Errorf("verify capability for %s %s: %v", request.Method, request.URL.Path, err)
		}
	}))
	defer server.Close()
	client, err := NewHTTPSandboxClient(server.URL, time.Second, signer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Validate(context.Background(), ValidationRequest{InstallationID: 10, RevisionID: 11, SourceType: "https"}); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if _, err := client.Execute(context.Background(), ExecutionRequest{ExecutionID: "mcp:signed"}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, err := client.LookupExecution(context.Background(), "mcp:signed"); err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(tokens) != 3 {
		t.Fatalf("signed token count=%d, want 3", len(tokens))
	}
}

func TestHTTPSandboxClientDoesNotForwardSecretAcrossRedirect(t *testing.T) {
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			const secret = "sandbox-one-time-wrap-token"
			var targetCalls atomic.Int32
			var targetBody atomic.Value
			target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
				targetCalls.Add(1)
				body, _ := io.ReadAll(request.Body)
				targetBody.Store(string(body))
			}))
			defer target.Close()
			redirect := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.Header().Set("Location", target.URL+"/capture")
				response.WriteHeader(status)
			}))
			defer redirect.Close()

			client, err := NewHTTPSandboxClient(redirect.URL, time.Second, testSandboxCapabilitySigner(t))
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Execute(context.Background(), ExecutionRequest{
				ExecutionID: "mcp:no-redirect", SecretWrapToken: secret,
			})
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("status %d", status)) {
				t.Fatalf("expected redirect status error, got %v", err)
			}
			if targetCalls.Load() != 0 {
				t.Fatalf("redirect target received %d request(s)", targetCalls.Load())
			}
			if captured := targetBody.Load(); captured != nil && strings.Contains(captured.(string), secret) {
				t.Fatal("redirect target received secret wrapping token")
			}
		})
	}
}

func testSandboxCapabilitySigner(t *testing.T) *SandboxCapabilitySigner {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewSandboxCapabilitySigner(privateKey, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}
