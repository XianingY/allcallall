package alerting

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type captureProvider struct {
	mu     sync.Mutex
	alerts []Alert
}

func (c *captureProvider) Notify(_ context.Context, a Alert) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.alerts = append(c.alerts, a)
	return nil
}

func (c *captureProvider) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.alerts)
}

func TestRoutingBySeverity(t *testing.T) {
	p1 := &captureProvider{}
	p2 := &captureProvider{}
	p3 := &captureProvider{}

	router := Routing{
		SeverityP1: {p1},
		SeverityP2: {p2},
		SeverityP3: {p3},
	}
	svc := NewService(router)

	if err := svc.Emit(context.Background(), Alert{Severity: SeverityP1, Title: "outage"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Emit(context.Background(), Alert{Severity: SeverityP3, Title: "ticket"}); err != nil {
		t.Fatal(err)
	}
	if p1.count() != 1 || p2.count() != 0 || p3.count() != 1 {
		t.Fatalf("routing wrong: p1=%d p2=%d p3=%d", p1.count(), p2.count(), p3.count())
	}
}

func TestDedupSuppressesDuplicates(t *testing.T) {
	p := &captureProvider{}
	svc := NewService(Routing{SeverityP3: {p}}, WithDedupWindow(time.Minute))

	a := Alert{Severity: SeverityP3, Title: "flap", Labels: map[string]string{"svc": "api"}}
	if err := svc.Emit(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if err := svc.Emit(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if p.count() != 1 {
		t.Fatalf("expected dedup to 1 notification, got %d", p.count())
	}
}

func TestWebhookProviderPostsJSON(t *testing.T) {
	var got Alert
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := WebhookProvider{URL: srv.URL}
	svc := NewService(Routing{SeverityP1: {p}})
	if err := svc.Emit(context.Background(), Alert{Severity: SeverityP1, Title: "page"}); err != nil {
		t.Fatal(err)
	}
	if got.Title != "page" || got.Severity != SeverityP1 {
		t.Fatalf("webhook received wrong payload: %+v", got)
	}
}
