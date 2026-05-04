package config

import (
	"encoding/json"
	"fmt"
	"io/fs"
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
	return resolveExtPath(entry, configDir)
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

	// Check for duplicate providers.
	providerSeen := make(map[string]bool, len(f.Core.Providers))

	for _, p := range f.Core.Providers {
		if providerSeen[p] {
			errs = append(errs, ValidationError{
				Field:   "core.providers",
				Message: fmt.Sprintf("duplicate provider %q", p),
			})

			break
		}

		providerSeen[p] = true
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
	if !IsPathEntry(entry) {
		validateBareExtName(errs, entry, field)

		return
	}

	if configDir == "" {
		return
	}

	validateExtPath(errs, entry, field, configDir)
}

func validateBareExtName(errs *ValidationErrors, entry, field string) {
	if !validName.MatchString(entry) {
		*errs = append(*errs, ValidationError{
			Field:   field,
			Message: fmt.Sprintf("invalid name %q (must match [a-zA-Z0-9_-]+)", entry),
		})
	}
}

func validateExtPath(errs *ValidationErrors, entry, field, configDir string) {
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

// resolveExtPath expands a path-like extension entry to an absolute path.
func resolveExtPath(entry, configDir string) (string, error) {
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

// hasGoFiles reports whether a directory contains at least one .go file,
// searching recursively into subdirectories.
func hasGoFiles(dir string) bool {
	found := false

	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil //nolint:nilerr // skip inaccessible entries during walk
		}

		if !d.IsDir() && strings.HasSuffix(d.Name(), ".go") && !strings.HasSuffix(d.Name(), "_test.go") {
			found = true

			return fs.SkipAll
		}

		return nil
	})

	return found
}
