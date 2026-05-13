package sdk

import (
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"

	"weave/internal/auth"
	"weave/sdk/registry"
)

type providerEntry struct {
	factory     func(Config) (Provider, error)
	authChecker func(Config) (bool, error)
}

var providerReg = registry.New[providerEntry](
	registry.WithWarn[providerEntry](log.New(os.Stderr, "weave: ", 0), "provider"),
)

// RegisterProvider registers a provider factory with typed configuration and auth structs.
// The framework will automatically populate the config struct from settings, env vars,
// and CLI flags before calling the factory. Auth is loaded from ~/.weave/auth.json
// and environment variables defined by the auth struct's env tags.
func RegisterProvider[TConfig, TAuth any](name string, factory func(Config, TConfig, TAuth) (Provider, error)) {
	var zeroConfig TConfig

	schema := extractSchema(reflect.TypeOf(zeroConfig))
	storeSchema("providers", name, schema)

	wrapper := func(cfg Config) (Provider, error) {
		var tc TConfig

		if err := cfg.ExtensionConfig("providers", name, &tc, ""); err != nil {
			return nil, fmt.Errorf("load provider config: %w", err)
		}

		var ta TAuth

		if err := auth.LoadProviderAuth(name, &ta); err != nil {
			return nil, fmt.Errorf("load provider auth: %w", err)
		}

		return factory(configOrDefault(cfg), tc, ta)
	}

	authChecker := makeAuthChecker[TAuth](name)

	providerReg.Register(name, providerEntry{factory: wrapper, authChecker: authChecker})
}

func makeAuthChecker[TAuth any](name string) func(Config) (bool, error) {
	return func(_ Config) (bool, error) {
		var ta TAuth

		if err := auth.LoadProviderAuth(name, &ta); err != nil {
			return false, fmt.Errorf("load provider auth: %w", err)
		}

		return hasAuthFieldSet(reflect.ValueOf(ta)), nil
	}
}

// tagContainsRule reports whether a struct tag value contains the given rule
// as a comma-separated entry. This handles validation tags like
// `validate:"required"` or `validate:"required,min=3"` without matching
// substrings like "notrequired".
func tagContainsRule(tag, rule string) bool {
	for part := range strings.SplitSeq(tag, ",") {
		if strings.TrimSpace(part) == rule {
			return true
		}
	}

	return false
}

// hasAuthFieldSet returns true if all exported fields tagged
// validate:"required" are non-zero. When no required fields are declared,
// it falls back to hasAnyFieldSet so single-field auth structs continue
// to work.
func hasAuthFieldSet(v reflect.Value) bool {
	if v.Kind() != reflect.Struct {
		return false
	}

	t := v.Type()
	hasRequired := false

	for i := range v.NumField() {
		if !t.Field(i).IsExported() {
			continue
		}

		field := v.Field(i)
		ft := t.Field(i)

		switch field.Kind() {
		case reflect.String:
			if tagContainsRule(ft.Tag.Get("validate"), "required") {
				hasRequired = true

				if field.String() == "" {
					return false
				}
			}
		case reflect.Pointer:
			if tagContainsRule(ft.Tag.Get("validate"), "required") {
				hasRequired = true

				if field.IsNil() {
					return false
				}
			}
		case reflect.Struct:
			nestedHasRequired := hasRequiredFieldTag(field)
			nestedOK := hasAuthFieldSet(field)

			if nestedHasRequired {
				hasRequired = true

				if !nestedOK {
					return false
				}
			}
		default:
			// Other field kinds are not considered for auth detection.
		}
	}

	if !hasRequired {
		return hasAnyFieldSet(v)
	}

	return true
}

// hasRequiredFieldTag reports whether any exported field in the struct
// (or its nested structs) carries a validate tag containing "required".
func hasRequiredFieldTag(v reflect.Value) bool {
	if v.Kind() != reflect.Struct {
		return false
	}

	t := v.Type()

	for i := range v.NumField() {
		if !t.Field(i).IsExported() {
			continue
		}

		field := v.Field(i)
		ft := t.Field(i)

		if tagContainsRule(ft.Tag.Get("validate"), "required") {
			return true
		}

		if field.Kind() == reflect.Struct && hasRequiredFieldTag(field) {
			return true
		}
	}

	return false
}

// hasAnyFieldSet returns true if at least one exported string or pointer field
// in the struct is non-zero. This handles multi-field auth structs where some
// fields may be optional.
func hasAnyFieldSet(v reflect.Value) bool {
	if v.Kind() != reflect.Struct {
		return false
	}

	t := v.Type()

	for i := range v.NumField() {
		if !t.Field(i).IsExported() {
			continue
		}

		field := v.Field(i)

		switch field.Kind() {
		case reflect.String:
			if field.String() != "" {
				return true
			}
		case reflect.Pointer:
			if !field.IsNil() {
				return true
			}
		case reflect.Struct:
			if hasAnyFieldSet(field) {
				return true
			}
		default:
			// Other field kinds are not considered for auth detection.
		}
	}

	return false
}

func ProviderRegistered(name string) bool {
	return providerReg.Exists(name)
}

func GetProvider(name string, cfg Config) (Provider, error) {
	entry, ok := providerReg.Get(name)
	if !ok {
		return nil, fmt.Errorf("provider %q not registered", name)
	}

	return entry.factory(configOrDefault(cfg))
}

// CheckProviderAuth returns whether the provider has auth credentials available
// (either in ~/.weave/auth.json or via environment variables).
func CheckProviderAuth(name string, cfg Config) (bool, error) {
	entry, ok := providerReg.Get(name)
	if !ok {
		return false, fmt.Errorf("provider %q not registered", name)
	}

	return entry.authChecker(configOrDefault(cfg))
}

func ListProviders() []string {
	return providerReg.List()
}

func ResetProviderRegistry() {
	providerReg.Reset()
	ResetSchemasForScope("providers")
}
