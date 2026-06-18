package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

var ErrToolArgumentsInvalid = errors.New("tool arguments invalid")

// ValidateToolArguments validates model-provided tool arguments against the registered strict JSON Schema.
func ValidateToolArguments(toolName, inputJSON string) error {
	descriptor, ok := ToolDescriptorByName(toolName)
	if !ok {
		return fmt.Errorf("%w: unknown tool %s", ErrToolArgumentsInvalid, toolName)
	}
	var value any
	if err := json.Unmarshal([]byte(inputJSON), &value); err != nil {
		return fmt.Errorf("%w: %v", ErrToolArgumentsInvalid, err)
	}
	if err := validateJSONValue("$", value, descriptor.InputSchema); err != nil {
		return fmt.Errorf("%w: %v", ErrToolArgumentsInvalid, err)
	}
	return nil
}

func validateJSONValue(path string, value any, schema any) error {
	schemaMap, ok := schema.(map[string]any)
	if !ok {
		if typed, ok := schema.(JSONSchema); ok {
			schemaMap = map[string]any(typed)
		} else {
			return nil
		}
	}
	if err := validateJSONType(path, value, schemaMap["type"]); err != nil {
		return err
	}
	if enumValues, ok := schemaMap["enum"].([]any); ok && len(enumValues) > 0 {
		matched := false
		for _, allowed := range enumValues {
			if fmt.Sprint(allowed) == fmt.Sprint(value) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s must be one of %v", path, enumValues)
		}
	}
	switch typeName(schemaMap["type"]) {
	case "object":
		return validateJSONObject(path, value, schemaMap)
	case "array":
		return validateJSONArray(path, value, schemaMap)
	default:
		return nil
	}
}

func validateJSONObject(path string, value any, schema map[string]any) error {
	object, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s must be object", path)
	}
	properties := map[string]any{}
	if raw, ok := schema["properties"].(map[string]any); ok {
		properties = raw
	}
	required := stringSet(schema["required"])
	for key := range required {
		if _, ok := object[key]; !ok {
			return fmt.Errorf("%s.%s is required", path, key)
		}
	}
	if additional, ok := schema["additionalProperties"].(bool); ok && !additional {
		for key := range object {
			if _, ok := properties[key]; !ok {
				return fmt.Errorf("%s.%s is not allowed", path, key)
			}
		}
	}
	for key, item := range object {
		propSchema, ok := properties[key]
		if !ok {
			continue
		}
		if err := validateJSONValue(path+"."+key, item, propSchema); err != nil {
			return err
		}
	}
	return nil
}

func validateJSONArray(path string, value any, schema map[string]any) error {
	array, ok := value.([]any)
	if !ok {
		return fmt.Errorf("%s must be array", path)
	}
	itemSchema, ok := schema["items"]
	if !ok {
		return nil
	}
	for i, item := range array {
		if err := validateJSONValue(fmt.Sprintf("%s[%d]", path, i), item, itemSchema); err != nil {
			return err
		}
	}
	return nil
}

func validateJSONType(path string, value any, rawType any) error {
	if rawType == nil {
		return nil
	}
	allowed := typeNames(rawType)
	if len(allowed) == 0 {
		return nil
	}
	for _, item := range allowed {
		if jsonValueMatchesType(value, item) {
			return nil
		}
	}
	return fmt.Errorf("%s must be %s", path, strings.Join(allowed, "|"))
}

func jsonValueMatchesType(value any, expected string) bool {
	switch expected {
	case "null":
		return value == nil
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "integer":
		number, ok := value.(float64)
		return ok && math.Trunc(number) == number
	case "number":
		_, ok := value.(float64)
		return ok
	default:
		return true
	}
}

func typeName(raw any) string {
	names := typeNames(raw)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func typeNames(raw any) []string {
	switch typed := raw.(type) {
	case string:
		return []string{typed}
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if value, ok := item.(string); ok {
				out = append(out, value)
			}
		}
		return out
	case []string:
		return typed
	default:
		return nil
	}
}

func stringSet(raw any) map[string]struct{} {
	out := map[string]struct{}{}
	switch typed := raw.(type) {
	case []string:
		for _, item := range typed {
			out[item] = struct{}{}
		}
	case []any:
		for _, item := range typed {
			if value, ok := item.(string); ok {
				out[value] = struct{}{}
			}
		}
	}
	return out
}
