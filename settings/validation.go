package settings

import (
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
	return fmt.Sprintf("settings.%s: %s", e.Field, e.Message)
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

// Validate checks a Settings for configuration errors.
func Validate(s *Settings) error {
	return ValidateWithConfigDir(s, "")
}

// ValidateWithConfigDir validates the config and resolves path-based extension
// entries relative to configDir. When configDir is empty, path existence
// checks are skipped.
func ValidateWithConfigDir(s *Settings, configDir string) error {
	var errs ValidationErrors

	if s.Output != "" && s.Output != "text" && s.Output != "json" {
		errs = append(errs, ValidationError{
			Field:   "output",
			Message: fmt.Sprintf("invalid value %q, must be \"text\" or \"json\"", s.Output),
		})
	}

	if s.Continue && s.Resume != "" {
		errs = append(errs, ValidationError{
			Field:   "continue",
			Message: "--continue and --resume are mutually exclusive",
		})
	}

	if len(errs) > 0 {
		return errs
	}

	return nil
}
