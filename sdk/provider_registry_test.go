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

	RegisterProvider[struct{}]("mock", func(Config, struct{}) (Provider, error) {
		return &ProviderMock{}, nil
	})

	got, err := GetProvider("mock", nil)
	require.NoError(t, err, "GetProvider")
	require.NotNil(t, got)
}

func TestDuplicateProviderRegistration(t *testing.T) {
	ResetProviderRegistry()

	RegisterProvider[struct{}]("dup", func(Config, struct{}) (Provider, error) {
		return &ProviderMock{}, nil
	})

	// Second registration should be a no-op with a warning (no panic).
	RegisterProvider[struct{}]("dup", func(Config, struct{}) (Provider, error) {
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

	RegisterProvider[struct{}]("fail", func(Config, struct{}) (Provider, error) {
		return nil, errors.New("factory error")
	})

	_, err := GetProvider("fail", nil)
	require.Error(t, err, "expected error from failing factory")
	assert.Equal(t, "factory error", err.Error())
}

func TestListProviders(t *testing.T) {
	ResetProviderRegistry()

	RegisterProvider[struct{}]("anthropic", func(Config, struct{}) (Provider, error) {
		return &ProviderMock{}, nil
	})
	RegisterProvider[struct{}]("openai", func(Config, struct{}) (Provider, error) {
		return &ProviderMock{}, nil
	})

	names := ListProviders()
	sort.Strings(names)

	assert.Equal(t, []string{"anthropic", "openai"}, names)
}

func TestResetProviderRegistry(t *testing.T) {
	ResetProviderRegistry()

	RegisterProvider[struct{}]("temp", func(Config, struct{}) (Provider, error) {
		return &ProviderMock{}, nil
	})

	ResetProviderRegistry()

	assert.Empty(t, ListProviders())
}
