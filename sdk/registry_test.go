package sdk

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterAndRetrieve(t *testing.T) {
	ResetExtensionRegistry()

	ext := NewExtensionFunc("test", func(bus Bus) error { return nil })

	RegisterExtension[struct{}]("test", func(Config, PreferenceReader, struct{}) (Extension, error) { return ext, nil })

	got, err := GetExtension("test", nil)
	require.NoError(t, err, "GetExtension")
	assert.Equal(t, "test", got.Name())
}

func TestRegisterExtensionWithScopeStoresSchemaInfoType(t *testing.T) {
	ResetExtensionRegistry()

	ResetSchemas()
	defer ResetSchemas()

	type extensionConfig struct {
		Enabled bool `json:"enabled" default:"true"`
	}

	RegisterExtensionWithScope("typed", "extensions", func(Config, PreferenceReader, extensionConfig) (Extension, error) {
		return NewExtensionFunc("typed", nil), nil
	})

	info := GetSchemaInfo("extensions", "typed")
	require.NotNil(t, info)
	assert.Equal(t, reflect.TypeFor[extensionConfig](), info.Type)
}

func TestDuplicateRegistration(t *testing.T) {
	ResetExtensionRegistry()

	RegisterExtension[struct{}]("dup", func(Config, PreferenceReader, struct{}) (Extension, error) {
		return NewExtensionFunc("dup", func(bus Bus) error { return nil }), nil
	})

	// Duplicate extension registration logs a warning; first registration wins.
	RegisterExtension[struct{}]("dup", func(Config, PreferenceReader, struct{}) (Extension, error) {
		return NewExtensionFunc("dup-v2", func(bus Bus) error { return nil }), nil
	})

	got, err := GetExtension("dup", nil)
	require.NoError(t, err)
	assert.Equal(t, "dup", got.Name(), "first registration should win")
}

func TestMissingExtension(t *testing.T) {
	ResetExtensionRegistry()

	_, err := GetExtension("nonexistent", nil)
	require.Error(t, err, "expected error for missing extension")
}

func TestGetExtension_FactoryError(t *testing.T) {
	ResetExtensionRegistry()

	RegisterExtension[struct{}]("fail", func(Config, PreferenceReader, struct{}) (Extension, error) {
		return nil, errors.New("boom")
	})

	_, err := GetExtension("fail", nil)
	require.Error(t, err, "expected error from failing factory")
	assert.Equal(t, "boom", err.Error())
}

func TestListExtensions(t *testing.T) {
	ResetExtensionRegistry()

	RegisterExtension[struct{}]("alpha", func(Config, PreferenceReader, struct{}) (Extension, error) {
		return NewExtensionFunc("alpha", nil), nil
	})
	RegisterExtension[struct{}]("beta", func(Config, PreferenceReader, struct{}) (Extension, error) { return NewExtensionFunc("beta", nil), nil })

	names := ListExtensions()
	sort.Strings(names)

	assert.Equal(t, []string{"alpha", "beta"}, names)
}

func TestExtensionRegistered(t *testing.T) {
	ResetExtensionRegistry()

	assert.False(t, ExtensionRegistered("test"), "unregistered extension should not be found")

	RegisterExtension[struct{}]("test", func(Config, PreferenceReader, struct{}) (Extension, error) {
		return NewExtensionFunc("test", nil), nil
	})

	assert.True(t, ExtensionRegistered("test"), "registered extension should be found")
	assert.False(t, ExtensionRegistered("other"), "different name should not be found")
}

func TestRegisterExtensionWithScopeAndWriter_ReceivesPreferenceWriter(t *testing.T) {
	ResetExtensionRegistry()

	cfg := &mockPrefStoreConfig{}

	var receivedWriter PreferenceWriter

	RegisterExtensionWithScopeAndWriter[struct{}]("privileged", "extensions", func(_ Config, pw PreferenceWriter, _ struct{}) (Extension, error) {
		receivedWriter = pw
		return NewExtensionFunc("privileged", nil), nil
	})

	ext, err := GetExtension("privileged", cfg)
	require.NoError(t, err)
	assert.Equal(t, "privileged", ext.Name())
	assert.NotNil(t, receivedWriter)
	require.NoError(t, receivedWriter.SaveProviderKey("test", "key"))

	extensionWriter, ok := receivedWriter.(ExtensionConfigWriter)
	require.True(t, ok, "privileged writer should expose scoped extension config writes")

	target := map[string]any{"enabled": true}
	require.NoError(t, extensionWriter.SaveExtensionConfig("extensions", "privileged", target))
	assert.Equal(t, "extensions", cfg.savedExtensionScope)
	assert.Equal(t, "privileged", cfg.savedExtensionName)
	assert.Equal(t, target, cfg.savedExtensionTarget)
}

func TestRegisterExtensionWithScopeAndWriter_FallsBackToNoop(t *testing.T) {
	ResetExtensionRegistry()

	var receivedWriter PreferenceWriter

	RegisterExtensionWithScopeAndWriter[struct{}]("fallback", "extensions", func(_ Config, pw PreferenceWriter, _ struct{}) (Extension, error) {
		receivedWriter = pw
		return NewExtensionFunc("fallback", nil), nil
	})

	_, err := GetExtension("fallback", nil)
	require.NoError(t, err)
	assert.NotNil(t, receivedWriter)

	_, ok := receivedWriter.(ExtensionConfigWriter)
	assert.False(t, ok, "fallback writer should not claim scoped config write support")
}

func TestLegacyExtensionRegistrationExposesRuntimeWrapper(t *testing.T) {
	ResetExtensionRegistry()

	bus := &BusMock{}
	subscribed := false
	closed := false

	RegisterExtension[struct{}]("legacy", func(Config, PreferenceReader, struct{}) (Extension, error) {
		return NewExtensionFuncWithClose("legacy", func(got Bus) error {
			subscribed = true
			assert.Same(t, bus, got)

			return nil
		}, func() error {
			closed = true

			return nil
		}), nil
	})

	runtimeExt, err := GetRuntimeExtension("legacy", nil)
	require.NoError(t, err)

	require.NoError(t, runtimeExt.Register(NewExtensionContext(RuntimeContextOptions{Bus: bus})))
	assert.True(t, subscribed)

	closer, ok := runtimeExt.(interface{ Close() error })
	require.True(t, ok)
	require.NoError(t, closer.Close())
	assert.True(t, closed)

	ext, err := GetExtension("legacy", nil)
	require.NoError(t, err)
	closed = false
	require.NoError(t, ext.Close())
	assert.True(t, closed)
}

func TestRegisterRuntimeExtensionCanBeResolvedAsLegacyExtension(t *testing.T) {
	ResetExtensionRegistry()

	type runtimeConfig struct {
		Enabled bool `json:"enabled" default:"true"`
	}

	cfg := &extensionConfigRecorder{}
	bus := &BusMock{}
	var receivedConfig runtimeConfig
	var receivedPrefs PreferenceReader
	var registeredCtx ExtensionContext
	closed := false

	RegisterRuntimeExtension("runtime", func(_ Config, prefs PreferenceReader, c runtimeConfig) (RuntimeExtension, error) {
		receivedPrefs = prefs
		receivedConfig = c

		return NewRuntimeExtensionFuncWithClose(func(ctx ExtensionContext) error {
			registeredCtx = ctx

			return nil
		}, func() error {
			closed = true

			return nil
		}), nil
	})

	ext, err := GetExtension("runtime", cfg)
	require.NoError(t, err)
	assert.Equal(t, "runtime", ext.Name())
	assert.True(t, receivedConfig.Enabled)
	assert.NotNil(t, receivedPrefs)

	require.NoError(t, ext.Subscribe(bus))
	require.NotNil(t, registeredCtx)
	assert.Same(t, bus, registeredCtx.Bus())
	assert.NotNil(t, registeredCtx.Hooks())
	assert.NotNil(t, registeredCtx.Tools())
	assert.NoError(t, registeredCtx.Config("extensions", "runtime", &runtimeConfig{}))
	_, err = registeredCtx.Exec(context.Background(), ExecRequest{Command: "echo"})
	assert.ErrorIs(t, err, ErrRuntimeCapabilityUnsupported)

	require.NoError(t, ext.Close())
	assert.True(t, closed)
	assert.Equal(t, []string{"extensions/runtime", "extensions/runtime"}, cfg.loaded)
}

func TestRegisterRuntimeExtensionWithScopeStoresSchemaInfoType(t *testing.T) {
	ResetExtensionRegistry()

	type runtimeConfig struct {
		Value string `json:"value" default:"ok"`
	}

	RegisterRuntimeExtensionWithScope("runtime-scoped", "guardian", func(Config, PreferenceReader, runtimeConfig) (RuntimeExtension, error) {
		return NewRuntimeExtensionFunc(nil), nil
	})

	info := GetSchemaInfo("guardian", "runtime-scoped")
	require.NotNil(t, info)
	assert.Equal(t, reflect.TypeFor[runtimeConfig](), info.Type)
}

type extensionConfigRecorder struct {
	loaded []string
}

func (c *extensionConfigRecorder) FilePath() string   { return "" }
func (c *extensionConfigRecorder) ProjectDir() string { return "" }
func (c *extensionConfigRecorder) ExtensionConfig(scope, name string, target any) error {
	c.loaded = append(c.loaded, scope+"/"+name)
	v := reflect.ValueOf(target)
	if v.Kind() == reflect.Ptr && !v.IsNil() {
		elem := v.Elem()
		if elem.Kind() == reflect.Struct {
			field := elem.FieldByName("Enabled")
			if field.IsValid() && field.CanSet() && field.Kind() == reflect.Bool {
				field.SetBool(true)
			}
		}
	}

	return nil
}
func (c *extensionConfigRecorder) IsHeadless() bool       { return true }
func (c *extensionConfigRecorder) RespectGitignore() bool { return true }
