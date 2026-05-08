package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var validName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// IsPathEntry reports whether an extension entry is a filesystem path
// (prefixed with ./, ../, /, or ~/).
func IsPathEntry(s string) bool {
	return strings.HasPrefix(s, "./") ||
		strings.HasPrefix(s, "../") ||
		strings.HasPrefix(s, "/") ||
		strings.HasPrefix(s, "~/")
}

// ValidExtName reports whether a string is a valid bare extension name.
func ValidExtName(s string) bool {
	return validName.MatchString(s)
}

// ResolveExtPath expands a path-like extension entry to an absolute path.
func ResolveExtPath(entry, configDir string) (string, error) {
	if strings.HasPrefix(entry, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}

		return filepath.Join(home, entry[2:]), nil
	}

	if filepath.IsAbs(entry) {
		return entry, nil
	}

	abs, err := filepath.Abs(filepath.Join(configDir, entry))
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}

	return abs, nil
}

// ValidationError is a single validation failure with a field path.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("config.%s: %s", e.Field, e.Message)
}

// ValidationErrors holds multiple validation failures.
type ValidationErrors []ValidationError

func (errs ValidationErrors) Error() string {
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Error()
	}

	return strings.Join(msgs, "; ")
}

// Validate checks a File for configuration errors. Path-based extension entries
// are not resolved (use ValidateWithConfigDir for full validation).
func Validate(f *File) error {
	return ValidateWithConfigDir(f, "")
}

// ValidateWithConfigDir validates the config and resolves path-based extension
// entries relative to configDir. When configDir is empty, path existence
// checks are skipped.
func ValidateWithConfigDir(f *File, configDir string) error {
	var errs ValidationErrors

	if f.UI != UIValueTUI && f.UI != UIValueNone {
		errs = append(errs, ValidationError{
			Field:   "ui",
			Message: fmt.Sprintf("invalid value %q, must be \"tui\" or \"none\"", f.UI),
		})
	}

	if strings.TrimSpace(f.Core.AgentLoop) == "" {
		errs = append(errs, ValidationError{
			Field:   "core.agent_loop",
			Message: "must not be empty",
		})
	}

	for i, name := range f.ExcludeExtensions {
		if !validName.MatchString(name) {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("exclude_extensions[%d]", i),
				Message: fmt.Sprintf("invalid extension name %q (must match [a-zA-Z0-9_-]+)", name),
			})
		}
	}

	for name, raw := range f.Providers {
		validateProviderEntry(&errs, name, raw)
	}

	if len(errs) > 0 {
		return errs
	}

	return nil
}

func validateProviderEntry(errs *ValidationErrors, name string, raw any) {
	m, ok := raw.(map[string]any)
	if !ok {
		*errs = append(*errs, ValidationError{
			Field:   "providers." + name,
			Message: fmt.Sprintf("expected object, got %T", raw),
		})

		return
	}

	jsonBytes, err := json.Marshal(m)
	if err != nil {
		*errs = append(*errs, ValidationError{
			Field:   "providers." + name,
			Message: fmt.Sprintf("invalid config: %v", err),
		})

		return
	}

	var entry ProviderEntry
	if err := json.Unmarshal(jsonBytes, &entry); err != nil {
		*errs = append(*errs, ValidationError{
			Field:   "providers." + name,
			Message: fmt.Sprintf("invalid config: %v", err),
		})
	}
}
