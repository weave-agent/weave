package sdk

import (
	"errors"
	"os"
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

	require.NoError(t, os.Unsetenv("TEST_PROVIDER2_API_KEY"))

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
