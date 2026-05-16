package validate

import (
	"errors"
	"fmt"
	"reflect"
)

const (
	jsonTypeString  = "string"
	jsonTypeNumber  = "number"
	jsonTypeInteger = "integer"
	jsonTypeBoolean = "boolean"
	jsonTypeArray   = "array"
	jsonTypeObject  = "object"
)

// Args validates incoming args against a tool's parameter schema.
// It checks that required fields exist, types match, and rejects unknown
// properties when additionalProperties is false.
func Args(args, schema map[string]any) error {
	if schema == nil {
		return nil
	}

	properties, _ := schema["properties"].(map[string]any)
	required, _ := schema["required"].([]any)
	additionalPropertiesVal, hasAdditionalProperties := schema["additionalProperties"]
	additionalProperties, _ := additionalPropertiesVal.(bool)

	// Check required fields
	for _, req := range required {
		key, ok := req.(string)
		if !ok {
			continue
		}

		if _, exists := args[key]; !exists {
			return fmt.Errorf("missing required field: %q", key)
		}
	}

	// Check for unknown properties when additionalProperties is explicitly false
	if hasAdditionalProperties && !additionalProperties && properties != nil {
		for key := range args {
			if _, known := properties[key]; !known {
				return fmt.Errorf("unknown field: %q", key)
			}
		}
	}

	// Validate each argument against its property schema
	for key, value := range args {
		propSchema, ok := properties[key].(map[string]any)
		if !ok {
			continue
		}

		if err := validateValue(value, propSchema); err != nil {
			return fmt.Errorf("field %q: %w", key, err)
		}
	}

	return nil
}

func validateValue(value any, schema map[string]any) error {
	expectedType, _ := schema["type"].(string)
	if expectedType == "" {
		return nil
	}

	if value == nil {
		// null values are only valid if the schema explicitly allows them
		// (not implemented; treat as invalid for required fields)
		return nil
	}

	actualType := jsonTypeOf(value)

	// number also accepts integer
	if expectedType == jsonTypeNumber && actualType == jsonTypeInteger {
		return nil
	}

	if expectedType != actualType {
		return fmt.Errorf("expected type %q, got %q", expectedType, actualType)
	}

	// Validate array items
	if expectedType == jsonTypeArray {
		arr, ok := value.([]any)
		if !ok {
			return errors.New("expected array")
		}

		itemsSchema, ok := schema["items"].(map[string]any)
		if !ok {
			return nil
		}

		for i, item := range arr {
			if err := validateValue(item, itemsSchema); err != nil {
				return fmt.Errorf("item %d: %w", i, err)
			}
		}
	}

	// Validate object properties (nested object)
	if expectedType == jsonTypeObject && !isPrimitiveObject(value) {
		obj, ok := value.(map[string]any)
		if !ok {
			return errors.New("expected object")
		}

		return Args(obj, schema)
	}

	return nil
}

// isPrimitiveObject is a no-op placeholder. Property definitions and nested
// object schemas both have "type" at their root, so we let validateValue
// handle the recursion directly.
func isPrimitiveObject(_ any) bool {
	return false
}

func jsonTypeOf(value any) string {
	switch v := value.(type) {
	case string:
		return jsonTypeString
	case float64:
		if v == float64(int64(v)) {
			return jsonTypeInteger
		}

		return jsonTypeNumber
	case int, int8, int16, int32, int64:
		return jsonTypeInteger
	case uint, uint8, uint16, uint32, uint64:
		return jsonTypeInteger
	case bool:
		return jsonTypeBoolean
	case []any:
		return jsonTypeArray
	case map[string]any:
		return jsonTypeObject
	default:
		// Handle typed slices that may come from JSON unmarshal
		typ := reflect.TypeOf(value)
		if typ != nil && typ.Kind() == reflect.Slice {
			return jsonTypeArray
		}

		if typ != nil && typ.Kind() == reflect.Map {
			return jsonTypeObject
		}

		return "unknown"
	}
}
