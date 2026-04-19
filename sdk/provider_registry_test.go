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

	RegisterProvider("mock", func(Config) (Provider, error) {
		return &ProviderMock{}, nil
	})

	got, err := GetProvider("mock", nil)
	require.NoError(t, err, "GetProvider")
	require.NotNil(t, got)
}

func TestDuplicateProviderRegistration(t *testing.T) {
	ResetProviderRegistry()

	RegisterProvider("dup", func(Config) (Provider, error) {
		return &ProviderMock{}, nil
	})

	defer func() {
		require.NotNil(t, recover(), "expected panic on duplicate provider registration")
	}()

	RegisterProvider("dup", func(Config) (Provider, error) {
		return &ProviderMock{}, nil
	})
}

func TestMissingProvider(t *testing.T) {
	ResetProviderRegistry()

	_, err := GetProvider("nonexistent", nil)
	require.Error(t, err, "expected error for missing provider")
}

func TestGetProvider_FactoryError(t *testing.T) {
	ResetProviderRegistry()

	RegisterProvider("fail", func(Config) (Provider, error) {
		return nil, errors.New("factory error")
	})

	_, err := GetProvider("fail", nil)
	require.Error(t, err, "expected error from failing factory")
	assert.Equal(t, "factory error", err.Error())
}

func TestListProviders(t *testing.T) {
	ResetProviderRegistry()

	RegisterProvider("anthropic", func(Config) (Provider, error) {
		return &ProviderMock{}, nil
	})
	RegisterProvider("openai", func(Config) (Provider, error) {
		return &ProviderMock{}, nil
	})

	names := ListProviders()
	sort.Strings(names)

	assert.Equal(t, []string{"anthropic", "openai"}, names)
}

func TestResetProviderRegistry(t *testing.T) {
	ResetProviderRegistry()

	RegisterProvider("temp", func(Config) (Provider, error) {
		return &ProviderMock{}, nil
	})

	ResetProviderRegistry()

	assert.Empty(t, ListProviders())
}
