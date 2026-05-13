package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"reflect"
	"strconv"
)

// LoadProviderAuth loads provider authentication into the target struct.
// It reads from ~/.weave/auth.json and environment variables (using env tags
// with no prefix, e.g. ANTHROPIC_API_KEY). Env vars override auth file values.
func LoadProviderAuth(providerName string, target any) error {
	if target == nil {
		return errors.New("target is nil")
	}

	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return fmt.Errorf("target must be a non-nil pointer, got %T", target)
	}

	if v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("target must point to a struct, got %T", target)
	}

	// Load auth file.
	authFile, err := Load()
	if err != nil {
		// Log warning but continue — env vars may still provide valid auth.
		log.Printf("weave: warning: failed to load auth file: %v", err)

		authFile = &File{Providers: make(map[string]json.RawMessage)}
	}

	// Apply data from auth file (raw JSON unmarshaled directly into target).
	if raw, ok := authFile.Providers[providerName]; ok && len(raw) > 0 {
		if uerr := json.Unmarshal(raw, target); uerr != nil {
			return fmt.Errorf("unmarshal provider auth: %w", uerr)
		}
	}

	// Apply env vars (no prefix — env tags resolve directly).
	if err := applyEnvToStruct(target); err != nil {
		return fmt.Errorf("apply env vars: %w", err)
	}

	return nil
}

// applyEnvToStruct overrides fields from environment variables using `env` struct tags.
// Env vars are looked up as the exact env tag value (no prefix).
func applyEnvToStruct(target any) error {
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return errors.New("target must be a non-nil pointer")
	}

	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return errors.New("target must point to a struct")
	}

	t := v.Type()
	for i := range v.NumField() {
		field := v.Field(i)
		if !field.CanSet() {
			continue
		}

		ft := t.Field(i)

		// Recurse into nested structs.
		if field.Kind() == reflect.Struct {
			if err := applyEnvToStruct(field.Addr().Interface()); err != nil {
				return err
			}

			continue
		}

		envTag := ft.Tag.Get("env")
		if envTag == "" {
			continue
		}

		val, ok := os.LookupEnv(envTag)
		if !ok || val == "" {
			continue
		}

		if err := setFieldFromString(field, val); err != nil {
			return fmt.Errorf("field %s env %s=%q: %w", ft.Name, envTag, val, err)
		}
	}

	return nil
}

// setFieldFromString sets a field value from a string representation.
func setFieldFromString(field reflect.Value, s string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return fmt.Errorf("parse int: %w", err)
		}

		field.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return fmt.Errorf("parse uint: %w", err)
		}

		field.SetUint(n)
	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return fmt.Errorf("parse bool: %w", err)
		}

		field.SetBool(b)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return fmt.Errorf("parse float: %w", err)
		}

		field.SetFloat(f)
	default:
		return fmt.Errorf("unsupported field kind: %s", field.Kind())
	}

	return nil
}
