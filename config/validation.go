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

// isPathEntry reports whether an extension entry is a filesystem path
// (prefixed with ./, ../, /, or ~/).
func isPathEntry(s string) bool {
	return strings.HasPrefix(s, "./") ||
		strings.HasPrefix(s, "../") ||
		strings.HasPrefix(s, "/") ||
		strings.HasPrefix(s, "~/")
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

	if f.UI != "tui" && f.UI != "none" {
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

	if len(f.Core.Providers) == 0 {
		errs = append(errs, ValidationError{
			Field:   "core.providers",
			Message: "must contain at least one provider",
		})
	}

	for i, p := range f.Core.Providers {
		if !validName.MatchString(p) {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("core.providers[%d]", i),
				Message: fmt.Sprintf("invalid provider name %q (must match [a-zA-Z0-9_-]+)", p),
			})
		}
	}

	for i, ext := range f.Extensions {
		validateExtEntry(&errs, ext, fmt.Sprintf("extensions[%d]", i), configDir)
	}

	for i, ext := range f.UIExtensions {
		validateExtEntry(&errs, ext, fmt.Sprintf("ui_extensions[%d]", i), configDir)
	}

	for name, raw := range f.Providers {
		validateProviderEntry(&errs, name, raw)
	}

	if len(errs) > 0 {
		return errs
	}

	return nil
}

func validateExtEntry(errs *ValidationErrors, entry, field, configDir string) {
	if isPathEntry(entry) {
		if configDir != "" {
			dir, err := resolveExtPath(entry, configDir)
			if err != nil {
				*errs = append(*errs, ValidationError{
					Field:   field,
					Message: fmt.Sprintf("path %q: %s", entry, err),
				})
				return
			}

			stat, statErr := os.Stat(dir)
			if statErr != nil {
				*errs = append(*errs, ValidationError{
					Field:   field,
					Message: fmt.Sprintf("path %q does not exist", entry),
				})
				return
			}

			if !stat.IsDir() {
				*errs = append(*errs, ValidationError{
					Field:   field,
					Message: fmt.Sprintf("path %q is not a directory", entry),
				})
				return
			}

			if !hasGoFiles(dir) {
				*errs = append(*errs, ValidationError{
					Field:   field,
					Message: fmt.Sprintf("path %q contains no .go files", entry),
				})
			}
		}

		return
	}

	if !validName.MatchString(entry) {
		*errs = append(*errs, ValidationError{
			Field:   field,
			Message: fmt.Sprintf("invalid name %q (must match [a-zA-Z0-9_-]+)", entry),
		})
	}
}

func validateProviderEntry(errs *ValidationErrors, name string, raw any) {
	m, ok := raw.(map[string]any)
	if !ok {
		*errs = append(*errs, ValidationError{
			Field:   fmt.Sprintf("providers.%s", name),
			Message: fmt.Sprintf("expected object, got %T", raw),
		})
		return
	}

	jsonBytes, err := json.Marshal(m)
	if err != nil {
		*errs = append(*errs, ValidationError{
			Field:   fmt.Sprintf("providers.%s", name),
			Message: fmt.Sprintf("invalid config: %v", err),
		})
		return
	}

	var entry ProviderEntry
	if err := json.Unmarshal(jsonBytes, &entry); err != nil {
		*errs = append(*errs, ValidationError{
			Field:   fmt.Sprintf("providers.%s", name),
			Message: fmt.Sprintf("invalid config: %v", err),
		})
	}
}

// resolveExtPath expands a path-like extension entry to an absolute path.
func resolveExtPath(entry, configDir string) (string, error) {
	if strings.HasPrefix(entry, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}

		return filepath.Join(home, entry[2:]), nil
	}

	if filepath.IsAbs(entry) {
		return entry, nil
	}

	return filepath.Abs(filepath.Join(configDir, entry))
}

// hasGoFiles reports whether a directory contains at least one .go file.
func hasGoFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}

	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			return true
		}
	}

	return false
}
