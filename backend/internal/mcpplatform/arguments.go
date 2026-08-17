package mcpplatform

import (
	"encoding/json"
	"fmt"
)

func validateMCPArguments(schemaJSON string, arguments map[string]any) error {
	var schema map[string]any
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
		return fmt.Errorf("%w: stored MCP input schema is invalid", ErrInvalidState)
	}
	if schemaType, _ := schema["type"].(string); schemaType != "" && schemaType != "object" {
		return fmt.Errorf("%w: MCP input schema root must be an object", ErrInvalidState)
	}
	for _, name := range schemaStringValues(schema["required"]) {
		if _, ok := arguments[name]; !ok {
			return fmt.Errorf("%w: missing required MCP argument %q", ErrInvalidInput, name)
		}
	}
	properties, _ := schema["properties"].(map[string]any)
	if additional, ok := schema["additionalProperties"].(bool); ok && !additional {
		for name := range arguments {
			if _, declared := properties[name]; !declared {
				return fmt.Errorf("%w: undeclared MCP argument %q", ErrInvalidInput, name)
			}
		}
	}
	for name, value := range arguments {
		rawProperty, ok := properties[name]
		if !ok {
			continue
		}
		property, ok := rawProperty.(map[string]any)
		if !ok {
			continue
		}
		expected, _ := property["type"].(string)
		if expected != "" && !mcpJSONTypeMatches(expected, value) {
			return fmt.Errorf("%w: MCP argument %q must be %s", ErrInvalidInput, name, expected)
		}
		if enums, ok := property["enum"].([]any); ok && len(enums) > 0 {
			matched := false
			for _, candidate := range enums {
				if fmt.Sprint(candidate) == fmt.Sprint(value) {
					matched = true
					break
				}
			}
			if !matched {
				return fmt.Errorf("%w: MCP argument %q is not an allowed value", ErrInvalidInput, name)
			}
		}
	}
	return nil
}

func schemaStringValues(value any) []string {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func mcpJSONTypeMatches(expected string, value any) bool {
	switch expected {
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		switch value.(type) {
		case float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
			return true
		default:
			return false
		}
	case "integer":
		switch number := value.(type) {
		case float64:
			return number == float64(int64(number))
		case float32:
			return number == float32(int64(number))
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return true
		default:
			return false
		}
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "null":
		return value == nil
	default:
		return false
	}
}
