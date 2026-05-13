package sdk

import (
	"errors"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterAndRetrieveProvider(t *testing.T) {
	ResetProviderRegistry()

	RegisterProvider[struct{}, struct{}]("mock", func(Config, struct{}, struct{}) (Provider, error) {
		return &ProviderMock{}, nil
	})

	got, err := GetProvider("mock", nil)
	require.NoError(t, err, "GetProvider")
	require.NotNil(t, got)
}

func TestDuplicateProviderRegistration(t *testing.T) {
	ResetProviderRegistry()

	RegisterProvider[struct{}, struct{}]("dup", func(Config, struct{}, struct{}) (Provider, error) {
		return &ProviderMock{}, nil
	})

	// Second registration should be a no-op with a warning (no panic).
	RegisterProvider[struct{}, struct{}]("dup", func(Config, struct{}, struct{}) (Provider, error) {
		return &ProviderMock{}, nil
	})

	// First registration wins.
	got, err := GetProvider("dup", nil)
	require.NoError(t, err)
	require.NotNil(t, got)
}

func TestMissingProvider(t *testing.T) {
	ResetProviderRegistry()

	_, err := GetProvider("nonexistent", nil)
	require.Error(t, err, "expected error for missing provider")
}

func TestGetProvider_FactoryError(t *testing.T) {
	ResetProviderRegistry()

	RegisterProvider[struct{}, struct{}]("fail", func(Config, struct{}, struct{}) (Provider, error) {
		return nil, errors.New("factory error")
	})

	_, err := GetProvider("fail", nil)
	require.Error(t, err, "expected error from failing factory")
	assert.Equal(t, "factory error", err.Error())
}

func TestListProviders(t *testing.T) {
	ResetProviderRegistry()

	RegisterProvider[struct{}, struct{}]("anthropic", func(Config, struct{}, struct{}) (Provider, error) {
		return &ProviderMock{}, nil
	})
	RegisterProvider[struct{}, struct{}]("openai", func(Config, struct{}, struct{}) (Provider, error) {
		return &ProviderMock{}, nil
	})

	names := ListProviders()
	sort.Strings(names)

	assert.Equal(t, []string{"anthropic", "openai"}, names)
}

func TestResetProviderRegistry(t *testing.T) {
	ResetProviderRegistry()

	RegisterProvider[struct{}, struct{}]("temp", func(Config, struct{}, struct{}) (Provider, error) {
		return &ProviderMock{}, nil
	})

	ResetProviderRegistry()

	assert.Empty(t, ListProviders())
}

func TestCheckProviderAuth_EnvVar(t *testing.T) {
	ResetProviderRegistry()

	type TestAuth struct {
		APIKey string `env:"TEST_PROVIDER_API_KEY"`
	}

	RegisterProvider[struct{}, TestAuth]("test-provider", func(Config, struct{}, TestAuth) (Provider, error) {
		return &ProviderMock{}, nil
	})

	t.Setenv("TEST_PROVIDER_API_KEY", "test-key")

	hasAuth, err := CheckProviderAuth("test-provider", nil)
	require.NoError(t, err)
	assert.True(t, hasAuth)
}

func TestCheckProviderAuth_Missing(t *testing.T) {
	ResetProviderRegistry()

	type TestAuth struct {
		APIKey string `env:"TEST_PROVIDER2_API_KEY"`
	}

	RegisterProvider[struct{}, TestAuth]("test-provider2", func(Config, struct{}, TestAuth) (Provider, error) {
		return &ProviderMock{}, nil
	})

	t.Setenv("TEST_PROVIDER2_API_KEY", "")

	hasAuth, err := CheckProviderAuth("test-provider2", nil)
	require.NoError(t, err)
	assert.False(t, hasAuth)
}

func TestCheckProviderAuth_Unregistered(t *testing.T) {
	ResetProviderRegistry()

	_, err := CheckProviderAuth("nonexistent", nil)
	require.Error(t, err)
}

func TestCheckProviderAuth_NonStructAuth(t *testing.T) {
	ResetProviderRegistry()

	RegisterProvider[struct{}, struct{}]("no-auth", func(Config, struct{}, struct{}) (Provider, error) {
		return &ProviderMock{}, nil
	})

	hasAuth, err := CheckProviderAuth("no-auth", nil)
	require.NoError(t, err)
	assert.False(t, hasAuth)
}

func TestCheckProviderAuth_RequiredFieldOnly(t *testing.T) {
	ResetProviderRegistry()

	type RequiredAuth struct {
		APIKey string `json:"api_key" env:"TEST_REQ_API_KEY" validate:"required"`
		OrgID  string `json:"org_id" env:"TEST_REQ_ORG_ID"`
	}

	RegisterProvider[struct{}, RequiredAuth]("req-auth", func(Config, struct{}, RequiredAuth) (Provider, error) {
		return &ProviderMock{}, nil
	})

	// Only optional field set — auth should be false.
	t.Setenv("TEST_REQ_ORG_ID", "org-123")

	hasAuth, err := CheckProviderAuth("req-auth", nil)
	require.NoError(t, err)
	assert.False(t, hasAuth, "optional field only should not count as authenticated")
}

func TestCheckProviderAuth_RequiredFieldSet(t *testing.T) {
	ResetProviderRegistry()

	type RequiredAuth struct {
		APIKey string `json:"api_key" env:"TEST_REQ2_API_KEY" validate:"required"`
		OrgID  string `json:"org_id" env:"TEST_REQ2_ORG_ID"`
	}

	RegisterProvider[struct{}, RequiredAuth]("req-auth2", func(Config, struct{}, RequiredAuth) (Provider, error) {
		return &ProviderMock{}, nil
	})

	// Required field set — auth should be true.
	t.Setenv("TEST_REQ2_API_KEY", "sk-key")

	hasAuth, err := CheckProviderAuth("req-auth2", nil)
	require.NoError(t, err)
	assert.True(t, hasAuth, "required field set should count as authenticated")
}

func TestCheckProviderAuth_MultipleRequiredFields_BothSet(t *testing.T) {
	ResetProviderRegistry()

	type MultiRequiredAuth struct {
		ClientID     string `json:"client_id" env:"TEST_MULTI_CLIENT_ID" validate:"required"`
		ClientSecret string `json:"client_secret" env:"TEST_MULTI_CLIENT_SECRET" validate:"required"`
	}

	RegisterProvider[struct{}, MultiRequiredAuth]("multi-req", func(Config, struct{}, MultiRequiredAuth) (Provider, error) {
		return &ProviderMock{}, nil
	})

	// Both required fields set — auth should be true.
	t.Setenv("TEST_MULTI_CLIENT_ID", "id-123")
	t.Setenv("TEST_MULTI_CLIENT_SECRET", "secret-456")

	hasAuth, err := CheckProviderAuth("multi-req", nil)
	require.NoError(t, err)
	assert.True(t, hasAuth, "all required fields set should count as authenticated")
}

func TestCheckProviderAuth_MultipleRequiredFields_OnlyOneSet(t *testing.T) {
	ResetProviderRegistry()

	type MultiRequiredAuth struct {
		ClientID     string `json:"client_id" env:"TEST_MULTI2_CLIENT_ID" validate:"required"`
		ClientSecret string `json:"client_secret" env:"TEST_MULTI2_CLIENT_SECRET" validate:"required"`
	}

	RegisterProvider[struct{}, MultiRequiredAuth]("multi-req2", func(Config, struct{}, MultiRequiredAuth) (Provider, error) {
		return &ProviderMock{}, nil
	})

	// Only one required field set — auth should be false.
	t.Setenv("TEST_MULTI2_CLIENT_ID", "id-123")

	hasAuth, err := CheckProviderAuth("multi-req2", nil)
	require.NoError(t, err)
	assert.False(t, hasAuth, "only one required field set should not count as authenticated")
}

func TestCheckProviderAuth_MultipleRequiredFields_NeitherSet(t *testing.T) {
	ResetProviderRegistry()

	type MultiRequiredAuth struct {
		ClientID     string `json:"client_id" env:"TEST_MULTI3_CLIENT_ID" validate:"required"`
		ClientSecret string `json:"client_secret" env:"TEST_MULTI3_CLIENT_SECRET" validate:"required"`
	}

	RegisterProvider[struct{}, MultiRequiredAuth]("multi-req3", func(Config, struct{}, MultiRequiredAuth) (Provider, error) {
		return &ProviderMock{}, nil
	})

	// No fields set — auth should be false.
	hasAuth, err := CheckProviderAuth("multi-req3", nil)
	require.NoError(t, err)
	assert.False(t, hasAuth, "no required fields set should not count as authenticated")
}

func TestCheckProviderAuth_BoolFieldSet(t *testing.T) {
	ResetProviderRegistry()

	type BoolAuth struct {
		Enabled bool `json:"enabled" env:"TEST_BOOL_ENABLED"`
	}

	RegisterProvider[struct{}, BoolAuth]("bool-auth", func(Config, struct{}, BoolAuth) (Provider, error) {
		return &ProviderMock{}, nil
	})

	t.Setenv("TEST_BOOL_ENABLED", "true")

	hasAuth, err := CheckProviderAuth("bool-auth", nil)
	require.NoError(t, err)
	assert.True(t, hasAuth, "bool field set to true should count as authenticated")
}

func TestCheckProviderAuth_BoolFieldNotSet(t *testing.T) {
	ResetProviderRegistry()

	type BoolAuth struct {
		Enabled bool `json:"enabled" env:"TEST_BOOL2_ENABLED"`
	}

	RegisterProvider[struct{}, BoolAuth]("bool-auth2", func(Config, struct{}, BoolAuth) (Provider, error) {
		return &ProviderMock{}, nil
	})

	hasAuth, err := CheckProviderAuth("bool-auth2", nil)
	require.NoError(t, err)
	assert.False(t, hasAuth, "bool field not set should not count as authenticated")
}

func TestCheckProviderAuth_IntFieldSet(t *testing.T) {
	ResetProviderRegistry()

	type IntAuth struct {
		Timeout int `json:"timeout" env:"TEST_INT_TIMEOUT"`
	}

	RegisterProvider[struct{}, IntAuth]("int-auth", func(Config, struct{}, IntAuth) (Provider, error) {
		return &ProviderMock{}, nil
	})

	t.Setenv("TEST_INT_TIMEOUT", "30")

	hasAuth, err := CheckProviderAuth("int-auth", nil)
	require.NoError(t, err)
	assert.True(t, hasAuth, "int field set to non-zero should count as authenticated")
}

func TestCheckProviderAuth_IntFieldZero(t *testing.T) {
	ResetProviderRegistry()

	type IntAuth struct {
		Timeout int `json:"timeout" env:"TEST_INT2_TIMEOUT"`
	}

	RegisterProvider[struct{}, IntAuth]("int-auth2", func(Config, struct{}, IntAuth) (Provider, error) {
		return &ProviderMock{}, nil
	})

	hasAuth, err := CheckProviderAuth("int-auth2", nil)
	require.NoError(t, err)
	assert.False(t, hasAuth, "int field at zero should not count as authenticated")
}

func TestCheckProviderAuth_RequiredBoolField(t *testing.T) {
	ResetProviderRegistry()

	type RequiredBoolAuth struct {
		Enabled bool `json:"enabled" env:"TEST_REQ_BOOL_ENABLED" validate:"required"`
	}

	RegisterProvider[struct{}, RequiredBoolAuth]("req-bool-auth", func(Config, struct{}, RequiredBoolAuth) (Provider, error) {
		return &ProviderMock{}, nil
	})

	// Required bool at false — auth should be false.
	hasAuth, err := CheckProviderAuth("req-bool-auth", nil)
	require.NoError(t, err)
	assert.False(t, hasAuth, "required bool field at false should not count as authenticated")

	// Required bool at true — auth should be true.
	t.Setenv("TEST_REQ_BOOL_ENABLED", "true")

	hasAuth, err = CheckProviderAuth("req-bool-auth", nil)
	require.NoError(t, err)
	assert.True(t, hasAuth, "required bool field at true should count as authenticated")
}

func TestCheckProviderAuth_PointerToStructField(t *testing.T) {
	ResetProviderRegistry()

	type Inner struct {
		APIKey string `json:"api_key" env:"TEST_PTR_INNER_API_KEY"`
	}

	type PtrStructAuth struct {
		Inner *Inner
	}

	RegisterProvider[struct{}, PtrStructAuth]("ptr-struct-auth", func(Config, struct{}, PtrStructAuth) (Provider, error) {
		return &ProviderMock{}, nil
	})

	t.Setenv("TEST_PTR_INNER_API_KEY", "ptr-key")

	hasAuth, err := CheckProviderAuth("ptr-struct-auth", nil)
	require.NoError(t, err)
	assert.True(t, hasAuth, "pointer to struct with set nested field should count as authenticated")
}
