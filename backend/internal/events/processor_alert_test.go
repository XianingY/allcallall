package events

import (
	"context"
	"errors"
	"testing"

	"github.com/allcallall/backend/internal/alerting"
	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
)

// capturingProvider 记录收到的告警，用于断言"告警真的被发出去了"。
type capturingProvider struct {
	alerts []alerting.Alert
}

func (c *capturingProvider) Notify(_ context.Context, a alerting.Alert) error {
	c.alerts = append(c.alerts, a)
	return nil
}

// 回归点（P0-2）：internal/alerting 此前整包零引用，P1/P2/P3 告警无人接收。
// 本测试锁死"outbox 批次失败必须上报告警"这一契约。
func TestProcessorRunFailureEmitsAlert(t *testing.T) {
	sink := &capturingProvider{}
	svc := alerting.NewService(alerting.Routing{alerting.SeverityP2: {sink}})

	store, _ := newProcessorTestStore(t)
	processor := NewProcessor(store, metrics.NewCounterStore()).WithAlerter(svc)

	processor.recordRunFailure(errors.New("simulated batch failure"))

	if len(sink.alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(sink.alerts))
	}
	got := sink.alerts[0]
	if got.Severity != alerting.SeverityP2 {
		t.Fatalf("severity=%s want P2", got.Severity)
	}
	if got.Title == "" {
		t.Fatal("alert title must be set")
	}
	if got.Labels["component"] != "outbox" {
		t.Fatalf("labels=%v want component=outbox", got.Labels)
	}
}

// 未注入告警服务时不得 panic（向后兼容）。
func TestProcessorRecordRunFailureWithoutAlerter(t *testing.T) {
	store, _ := newProcessorTestStore(t)
	processor := NewProcessor(store, metrics.NewCounterStore())
	processor.recordRunFailure(errors.New("boom"))
	_ = models.EventOutbox{}
}
