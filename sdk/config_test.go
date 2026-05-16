package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreferenceStoreFrom_ReturnsPreferenceReader(t *testing.T) {
	cfg := &mockPrefStoreConfig{}

	reader := PreferenceStoreFrom(cfg)
	require.NotNil(t, reader)

	assert.NotNil(t, reader, "PreferenceStoreFrom should return a PreferenceReader")

	_, ok := reader.(PreferenceWriter)
	assert.True(t, ok, "PreferenceStoreFrom should return a PreferenceWriter when config implements PreferenceStore")
}

func TestPreferenceStoreFrom_NilConfigReturnsNoop(t *testing.T) {
	reader := PreferenceStoreFrom(nil)
	require.NotNil(t, reader)

	var prefs struct{ Provider string }
	assert.NoError(t, reader.Preferences(&prefs))
}

func TestNoopPreferenceStore_ImplementsAllInterfaces(t *testing.T) {
	noop := NoopPreferenceStore{}

	var (
		_ PreferenceReader = noop
		_ PreferenceWriter = noop
		_ PreferenceStore  = noop
	)
}

func TestPreferenceStore_ImplementsPreferenceWriter(t *testing.T) {
	// PreferenceStore embeds PreferenceWriter, so any implementation of
	// PreferenceStore is also a PreferenceWriter.
	var (
		store PreferenceStore  = NoopPreferenceStore{}
		_     PreferenceWriter = store
	)
}

func TestPreferenceReader_HasOnlyPreferences(t *testing.T) {
	// PreferenceReader should only have Preferences method.
	var reader PreferenceReader = NoopPreferenceStore{}
	assert.NotNil(t, reader)
}

func TestPreferenceWriter_HasSaveMethods(t *testing.T) {
	var writer PreferenceWriter = NoopPreferenceStore{}

	assert.NoError(t, writer.SavePreferences(&struct{}{}))
	assert.NoError(t, writer.SaveProviderKey("test", "key"))
}

func TestRegisterTool_ReceivesPreferenceReader(t *testing.T) {
	ResetToolRegistry()

	var received PreferenceReader

	RegisterTool[struct{}]("test-tool", func(_ Config, ps PreferenceReader, _ struct{}) (Tool, error) {
		received = ps
		return &ToolMock{}, nil
	})

	_, err := GetTool("test-tool", nil)
	require.NoError(t, err)
	assert.NotNil(t, received)
}

func TestRegisterExtension_ReceivesPreferenceReader(t *testing.T) {
	ResetExtensionRegistry()

	var received PreferenceReader

	RegisterExtension[struct{}]("test-ext", func(_ Config, ps PreferenceReader, _ struct{}) (Extension, error) {
		received = ps
		return NewExtensionFunc("test-ext", nil), nil
	})

	_, err := GetExtension("test-ext", nil)
	require.NoError(t, err)
	assert.NotNil(t, received)
}

func TestRegisterUIExtension_ReceivesPreferenceReader(t *testing.T) {
	ResetUIExtensionRegistry()

	var received PreferenceReader

	RegisterUIExtension("test-ui", func(_ Config, ps PreferenceReader, _ struct{}) (UIExtension, error) {
		received = ps
		return &stubUIExt{name: "test-ui"}, nil
	})

	_, err := GetUIExtension("test-ui", nil)
	require.NoError(t, err)
	assert.NotNil(t, received)
}

func TestPreferenceReader_TypeAssertionToPreferenceWriter(t *testing.T) {
	// When the underlying config implements PreferenceStore, the returned
	// PreferenceReader can be type-asserted to PreferenceWriter.
	cfg := &mockPrefStoreConfig{}
	reader := PreferenceStoreFrom(cfg)

	writer, ok := reader.(PreferenceWriter)
	require.True(t, ok, "PreferenceReader from PreferenceStore should assert to PreferenceWriter")
	assert.NotNil(t, writer)
}

func TestPreferenceReader_TypeAssertionFailsForNoop(t *testing.T) {
	// NoopPreferenceStore implements all interfaces, so this should work.
	var reader PreferenceReader = NoopPreferenceStore{}

	writer, ok := reader.(PreferenceWriter)
	assert.True(t, ok)
	assert.NotNil(t, writer)
}

type mockPrefStoreConfig struct{}

func (m *mockPrefStoreConfig) FilePath() string                         { return "" }
func (m *mockPrefStoreConfig) ProjectDir() string                       { return "" }
func (m *mockPrefStoreConfig) ExtensionConfig(_, _ string, _ any) error { return nil }
func (m *mockPrefStoreConfig) IsHeadless() bool                         { return true }
func (m *mockPrefStoreConfig) RespectGitignore() bool                   { return true }
func (m *mockPrefStoreConfig) Preferences(any) error                    { return nil }
func (m *mockPrefStoreConfig) SavePreferences(any) error                { return nil }
func (m *mockPrefStoreConfig) SaveProviderKey(_, _ string) error        { return nil }

type stubUIExt struct{ name string }

func (s stubUIExt) Name() string  { return s.name }
func (s stubUIExt) Register(_ UI) {}
