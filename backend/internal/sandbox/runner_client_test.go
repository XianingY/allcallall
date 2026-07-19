package sandbox

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/allcallall/backend/internal/mcpplatform"
)

func TestHTTPRunnerDoesNotForwardSecretAcrossRedirect(t *testing.T) {
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			const secret = "runner-one-time-wrap-token"
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

			runner, err := NewHTTPRunner(redirect.URL)
			if err != nil {
				t.Fatal(err)
			}
			_, err = runner.Execute(context.Background(), mcpplatform.ExecutionRequest{
				ExecutionID: "mcp:no-runner-redirect", SecretWrapToken: secret,
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

func TestHTTPRunnerRejectsUnsafeBaseURL(t *testing.T) {
	for _, baseURL := range []string{
		"ftp://runner.example.com",
		"file:///tmp/runner.sock",
		"https://user:password@runner.example.com",
		"runner.example.com",
	} {
		t.Run(baseURL, func(t *testing.T) {
			if _, err := NewHTTPRunner(baseURL); err == nil {
				t.Fatal("expected unsafe runner base URL rejection")
			}
		})
	}
}
