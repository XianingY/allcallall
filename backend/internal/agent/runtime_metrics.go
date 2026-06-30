package agent

import "strings"

func (s *Service) RecordRAGRuntimeBridgeQuery(toolName string) {
	if s == nil || s.metrics == nil {
		return
	}
	s.metrics.Inc("rag_runtime_query_total")
	name := strings.TrimSpace(toolName)
	if name != "" {
		s.metrics.Inc("rag_runtime_bridge_" + sanitizeMetricSuffix(name) + "_total")
	}
}

func sanitizeMetricSuffix(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}
	return strings.Trim(builder.String(), "_")
}
