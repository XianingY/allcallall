package trace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultOTLPTimeout = 2 * time.Second
	defaultServiceName = "allcallall-backend"
)

type OTLPHTTPSpanRecorder struct {
	endpoint    string
	serviceName string
	client      *http.Client
}

func NewOTLPHTTPSpanRecorder(endpoint, serviceName string) *OTLPHTTPSpanRecorder {
	endpoint = normalizeOTLPEndpoint(endpoint)
	if endpoint == "" {
		return nil
	}
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		serviceName = defaultServiceName
	}
	return &OTLPHTTPSpanRecorder{
		endpoint:    endpoint,
		serviceName: serviceName,
		client:      &http.Client{Timeout: defaultOTLPTimeout},
	}
}

func NewOTLPHTTPSpanRecorderFromEnv() *OTLPHTTPSpanRecorder {
	return NewOTLPHTTPSpanRecorder(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"), os.Getenv("OTEL_SERVICE_NAME"))
}

func (r *OTLPHTTPSpanRecorder) RecordSpan(record SpanRecord) {
	if r == nil || r.endpoint == "" {
		return
	}
	payload := r.buildPayload(record)
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

func (r *OTLPHTTPSpanRecorder) buildPayload(record SpanRecord) map[string]any {
	attributes := []map[string]any{
		otlpStringAttribute("request_id", record.RequestID),
		otlpStringAttribute("span.status", record.Status),
	}
	if record.OutboxID != 0 {
		attributes = append(attributes, otlpStringAttribute("outbox_id", record.OutboxID))
	}
	if record.Error != "" {
		attributes = append(attributes, otlpStringAttribute("error", record.Error))
	}
	for key, value := range record.Attributes {
		attributes = append(attributes, otlpStringAttribute(key, value))
	}
	attributes = compactOTLPAttributes(attributes)
	statusCode := 1
	if record.Status == "error" {
		statusCode = 2
	}
	return map[string]any{
		"resourceSpans": []any{
			map[string]any{
				"resource": map[string]any{
					"attributes": []any{
						otlpStringAttribute("service.name", r.serviceName),
					},
				},
				"scopeSpans": []any{
					map[string]any{
						"scope": map[string]any{
							"name":    "allcallall-lite-trace",
							"version": "1",
						},
						"spans": []any{
							map[string]any{
								"traceId":           otlpTraceID(record.TraceID),
								"spanId":            otlpSpanID(record.SpanID),
								"parentSpanId":      otlpSpanID(record.ParentSpanID),
								"name":              record.Name,
								"kind":              1,
								"startTimeUnixNano": unixNanoString(record.StartedAt),
								"endTimeUnixNano":   unixNanoString(record.EndedAt),
								"attributes":        attributes,
								"status": map[string]any{
									"code":    statusCode,
									"message": record.Error,
								},
							},
						},
					},
				},
			},
		},
	}
}

func normalizeOTLPEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	if strings.HasSuffix(parsed.Path, "/v1/traces") {
		return parsed.String()
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/v1/traces"
	return parsed.String()
}

func otlpTraceID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:16])
}

func otlpSpanID(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func otlpStringAttribute(key string, value any) map[string]any {
	if key == "" || value == nil {
		return nil
	}
	stringValue := strings.TrimSpace(toAttributeString(value))
	if stringValue == "" {
		return nil
	}
	return map[string]any{
		"key": key,
		"value": map[string]any{
			"stringValue": stringValue,
		},
	}
}

func compactOTLPAttributes(attributes []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(attributes))
	for _, item := range attributes {
		if item != nil {
			out = append(out, item)
		}
	}
	return out
}

func unixNanoString(value time.Time) string {
	if value.IsZero() {
		return "0"
	}
	return strconv.FormatInt(value.UTC().UnixNano(), 10)
}

func toAttributeString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case uint64:
		return strconv.FormatUint(v, 10)
	case int:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(raw)
	}
}
