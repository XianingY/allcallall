package agent

import (
	"encoding/json"
	"strings"

	"github.com/allcallall/backend/internal/models"
)

func joinMessageBodies(messages []models.Message) string {
	items := make([]string, 0, len(messages))
	for _, message := range messages {
		items = append(items, message.Body)
	}
	return strings.Join(items, " ")
}

func CompactSnippet(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func UniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func mustJSONString(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func decodeStringSlice(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []string{}
	}
	return out
}

func extractCallIDsFromMessages(messages []models.Message) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, message := range messages {
		if message.Type != models.MessageTypeCallEvent || strings.TrimSpace(message.MetadataJSON) == "" {
			continue
		}
		var metadata map[string]any
		if err := json.Unmarshal([]byte(message.MetadataJSON), &metadata); err != nil {
			continue
		}
		callID, _ := metadata["call_id"].(string)
		callID = strings.TrimSpace(callID)
		if callID == "" {
			continue
		}
		if _, ok := seen[callID]; ok {
			continue
		}
		seen[callID] = struct{}{}
		out = append(out, callID)
	}
	return out
}
