package sdk

import (
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"

	"github.com/weave-agent/weave/internal/auth"
	"github.com/weave-agent/weave/sdk/registry"
)

type providerEntry struct {
	factory     func(Config) (Provider, error)
	authChecker func() (bool, error)
}

var providerReg = registry.New[providerEntry](
	registry.WithWarn[providerEntry](func(name string) {
		slog.Warn("duplicate registration", "name", name, "kind", "provider")
	}),
)

// RegisterProvider registers a provider factory with typed configuration and auth structs.
// The framework will automatically populate the config struct from settings, env vars,
// and CLI flags before calling the factory. Auth is loaded from ~/.weave/auth.json
// and environment variables defined by the auth struct's env tags.
func RegisterProvider[TConfig, TAuth any](name string, factory func(Config, TConfig, TAuth) (Provider, error)) {
	var zeroConfig TConfig

	typ := reflect.TypeOf(zeroConfig)
	schema := extractSchema(typ)
	storeSchema("providers", name, schema, typ)

	wrapper := func(cfg Config) (Provider, error) {
		var tc TConfig

		if err := cfg.ExtensionConfig("providers", name, &tc); err != nil {
			return nil, fmt.Errorf("load provider config: %w", err)
		}

		var ta TAuth

		if err := auth.LoadProviderAuth(name, &ta); err != nil {
			return nil, fmt.Errorf("load provider auth: %w", err)
		}

		return factory(ConfigReadOnly(cfg), tc, ta)
	}

	authChecker := makeAuthChecker[TAuth](name)

	providerReg.Register(name, providerEntry{factory: wrapper, authChecker: authChecker})
}

func makeAuthChecker[TAuth any](name string) func() (bool, error) {
	return func() (bool, error) {
		var ta TAuth

		if err := auth.LoadProviderAuth(name, &ta); err != nil {
			return false, fmt.Errorf("load provider auth: %w", err)
		}

		if hasAuthFieldSet(reflect.ValueOf(ta)) {
			return true, nil
		}

		// Fallback: check auth file for OAuth credentials for providers that
		// explicitly support OAuth. This handles providers whose auth struct
		// doesn't yet declare an OAuthToken field but are registered in the
		// OAuth provider registry.
		_, isOAuthProvider := GetOAuthProvider(name)
		if !isOAuthProvider {
			return false, nil
		}

		authFile, err := auth.Load()
		if err != nil {
			return false, fmt.Errorf("load auth file for oauth fallback: %w", err)
		}

		// Only check for OAuth access_token, not API keys. API-key auth
		// should be declared in the provider's auth struct.
		// Treat OAuth auth as usable only when the access token is not
		// expired, or when a refresh token is present.
		cred := authFile.GetOAuthCredential(name)

		return cred.AccessToken != "" && (!cred.IsExpired() || cred.RefreshToken != ""), nil
	}
}

// tagContainsRequired reports whether a struct tag value contains
// "required" as a comma-separated entry. This handles validation tags like
// `validate:"required"` or `validate:"required,min=3"` without matching
// substrings like "notrequired".
func tagContainsRequired(tag string) bool {
	for part := range strings.SplitSeq(tag, ",") {
		if strings.TrimSpace(part) == "required" {
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

		isRequired, isSet := checkFieldRequiredAndValue(field, ft)

		if isRequired {
			hasRequired = true

			if !isSet {
				return false
			}
		}
	}

	if !hasRequired {
		return hasAnyFieldSet(v)
	}

	return true
}

// checkFieldRequiredAndValue reports whether the field has a validate:"required"
// tag and whether its value is non-zero. For nested structs it checks recursively.
func checkFieldRequiredAndValue(field reflect.Value, ft reflect.StructField) (isRequired, isSet bool) {
	switch field.Kind() {
	case reflect.String:
		isRequired = tagContainsRequired(ft.Tag.Get("validate"))
		isSet = field.String() != ""
	case reflect.Bool:
		isRequired = tagContainsRequired(ft.Tag.Get("validate"))
		isSet = field.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		isRequired = tagContainsRequired(ft.Tag.Get("validate"))
		isSet = field.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		isRequired = tagContainsRequired(ft.Tag.Get("validate"))
		isSet = field.Uint() != 0
	case reflect.Float32, reflect.Float64:
		isRequired = tagContainsRequired(ft.Tag.Get("validate"))
		isSet = field.Float() != 0
	case reflect.Pointer:
		isRequired = tagContainsRequired(ft.Tag.Get("validate"))

		switch {
		case field.IsNil():
			isSet = false
		case field.Elem().Kind() == reflect.Struct:
			nestedHasRequired := hasRequiredFieldTag(field.Elem())
			nestedOK := hasAuthFieldSet(field.Elem())

			if nestedHasRequired {
				isRequired = true
				isSet = nestedOK
			} else {
				isSet = hasAnyFieldSet(field.Elem())
			}
		default:
			isSet = fieldIsSet(field.Elem())
		}
	case reflect.Struct:
		nestedHasRequired := hasRequiredFieldTag(field)
		nestedOK := hasAuthFieldSet(field)

		if nestedHasRequired {
			isRequired = true
			isSet = nestedOK
		}
	default:
		// Other field kinds are not considered for auth detection.
	}

	return isRequired, isSet
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

		if tagContainsRequired(ft.Tag.Get("validate")) {
			return true
		}

		if field.Kind() == reflect.Pointer && !field.IsNil() && field.Elem().Kind() == reflect.Struct {
			if hasRequiredFieldTag(field.Elem()) {
				return true
			}
		}

		if field.Kind() == reflect.Struct && hasRequiredFieldTag(field) {
			return true
		}
	}

	return false
}

// fieldIsSet reports whether a single reflect.Value is non-zero for auth
// detection purposes.
func fieldIsSet(field reflect.Value) bool {
	switch field.Kind() {
	case reflect.String:
		return field.String() != ""
	case reflect.Bool:
		return field.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return field.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return field.Uint() != 0
	case reflect.Float32, reflect.Float64:
		return field.Float() != 0
	case reflect.Pointer:
		if field.IsNil() {
			return false
		}

		if field.Elem().Kind() == reflect.Struct {
			return hasAnyFieldSet(field.Elem())
		}

		return fieldIsSet(field.Elem())
	case reflect.Struct:
		return hasAnyFieldSet(field)
	default:
		// Other field kinds are not considered for auth detection.
		return false
	}
}

// hasAnyFieldSet returns true if at least one exported field in the struct is
// non-zero. For OAuthCredential structs, only AccessToken is considered for
// auth detection; a refresh token alone does not make the provider usable.
func hasAnyFieldSet(v reflect.Value) bool {
	if v.Kind() != reflect.Struct {
		return false
	}

	if v.Type() == reflect.TypeFor[auth.OAuthCredential]() {
		accessTokenField := v.FieldByName("AccessToken")
		if !accessTokenField.IsValid() || accessTokenField.String() == "" {
			return false
		}

		// Token is usable if not expired, or if a refresh token is present
		// to obtain a new access token.
		cred := auth.OAuthCredential{AccessToken: accessTokenField.String()}

		refreshTokenField := v.FieldByName("RefreshToken")
		if refreshTokenField.IsValid() {
			cred.RefreshToken = refreshTokenField.String()
		}

		expiresAtField := v.FieldByName("ExpiresAt")
		if expiresAtField.IsValid() {
			cred.ExpiresAt = expiresAtField.Interface().(time.Time)
		}

		return !cred.IsExpired() || cred.RefreshToken != ""
	}

	t := v.Type()

	for i := range v.NumField() {
		if !t.Field(i).IsExported() {
			continue
		}

		if fieldIsSet(v.Field(i)) {
			return true
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
		return nil, fmt.Errorf("provider %q: %w", name, ErrNotRegistered)
	}

	return entry.factory(ConfigOrDefault(cfg))
}

// CheckProviderAuth returns whether the provider has auth credentials available
// (either in ~/.weave/auth.json or via environment variables).
func CheckProviderAuth(name string) (bool, error) {
	entry, ok := providerReg.Get(name)
	if !ok {
		return false, fmt.Errorf("provider %q: %w", name, ErrNotRegistered)
	}

	return entry.authChecker()
}

func ListProviders() []string {
	return providerReg.List()
}

func ResetProviderRegistry() {
	providerReg.Reset()
	ResetSchemasForScope("providers")
}
