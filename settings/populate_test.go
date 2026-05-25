package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weave-agent/weave/sdk"
)

type defaultsBuildNested struct {
	Mode string `json:"mode,omitempty" default:"fast"`
}

type defaultsBuildConfig struct {
	Name    string              `json:"name,omitempty" default:"weave"`
	Timeout int                 `json:"timeout,omitempty" default:"30"`
	Enabled bool                `json:"enabled,omitempty" default:"true"`
	Nested  defaultsBuildNested `json:"nested,omitzero"`
	Limit   *int                `json:"limit,omitempty" default:"5"`
}

func TestBuildDefaultsMap(t *testing.T) {
	got, err := buildDefaultsMap(&sdk.SchemaInfo{Type: reflect.TypeFor[defaultsBuildConfig]()})
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"name":    "weave",
		"timeout": float64(30),
		"enabled": true,
		"nested": map[string]any{
			removedSandboxModeKey: "fast",
		},
		"limit": float64(5),
	}, got)
}

func TestBuildDefaultsMapPointerType(t *testing.T) {
	got, err := buildDefaultsMap(&sdk.SchemaInfo{Type: reflect.TypeFor[*defaultsBuildConfig]()})
	require.NoError(t, err)

	assert.Equal(t, "weave", got["name"])
}

func TestBuildDefaultsMapRejectsMissingType(t *testing.T) {
	_, err := buildDefaultsMap(&sdk.SchemaInfo{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema type is nil")
}

func TestMergeMissing(t *testing.T) {
	defaults := map[string]any{
		"timeout":             float64(120),
		removedSandboxModeKey: "default",
		"nested": map[string]any{
			"added":     true,
			"preserved": "default",
		},
		"object": map[string]any{
			"from_default": true,
		},
	}
	existing := map[string]any{
		removedSandboxModeKey: "custom",
		"nested": map[string]any{
			"preserved": "custom",
		},
		"object": "custom-non-map",
	}

	got := mergeMissing(defaults, existing)

	assert.Equal(t, map[string]any{
		"timeout":             float64(120),
		removedSandboxModeKey: "custom",
		"nested": map[string]any{
			"added":     true,
			"preserved": "custom",
		},
		"object": "custom-non-map",
	}, got)
	assert.Equal(t, "custom", existing[removedSandboxModeKey], "existing map should not be mutated at top level")
}

func TestMapsEqual(t *testing.T) {
	tests := []struct {
		name string
		a    map[string]any
		b    map[string]any
		want bool
	}{
		{
			name: "equal",
			a: map[string]any{
				"name": "weave",
				"nested": map[string]any{
					"enabled": true,
				},
			},
			b: map[string]any{
				"name": "weave",
				"nested": map[string]any{
					"enabled": true,
				},
			},
			want: true,
		},
		{
			name: "different values",
			a:    map[string]any{"timeout": float64(30)},
			b:    map[string]any{"timeout": float64(60)},
			want: false,
		},
		{
			name: "different keys",
			a:    map[string]any{"timeout": float64(30)},
			b:    map[string]any{"timeout": float64(30), removedSandboxModeKey: "fast"},
			want: false,
		},
		{
			name: "different nested",
			a:    map[string]any{"nested": map[string]any{"enabled": true}},
			b:    map[string]any{"nested": map[string]any{"enabled": false}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, mapsEqual(tt.a, tt.b))
		})
	}
}

func TestPopulateExtensionDefaultsWritesMissingDefaults(t *testing.T) {
	sdk.ResetSchemas()
	defer sdk.ResetSchemas()

	sdk.RegisterExtensionSchema(configScopeTools, "test-tool", defaultsBuildConfig{})

	dir := t.TempDir()
	path := filepath.Join(dir, ".weave", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(`{"tools":{"test-tool":{"name":"custom","nested":{"extra":"kept"}}}}`), 0o600))

	require.NoError(t, populateExtensionDefaults(path, configScopeTools, "test-tool"))

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, map[string]any{
		configScopeTools: map[string]any{
			"test-tool": map[string]any{
				"name":    "custom",
				"timeout": float64(30),
				"enabled": true,
				"nested": map[string]any{
					"extra":               "kept",
					removedSandboxModeKey: "fast",
				},
				"limit": float64(5),
			},
		},
	}, got)
}

func TestExtensionConfigPopulatesDefaultsIdempotentlyAndPreservesCustomValues(t *testing.T) {
	sdk.ResetSchemas()
	defer sdk.ResetSchemas()

	sdk.RegisterExtensionSchema(configScopeTools, "test-tool", defaultsBuildConfig{})

	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".weave"), 0o750))

	projectDir := t.TempDir()
	sourcePath := filepath.Join(projectDir, ".weave", "settings.json")
	writeFile(t, projectDir, ".weave/settings.json", `{}`)

	cfg := &FullConfig{
		filePath: sourcePath,
		settings: mustLoadSettings(t, sourcePath),
	}

	var target defaultsBuildConfig
	require.NoError(t, cfg.ExtensionConfig(configScopeTools, "test-tool", &target))

	data, err := os.ReadFile(sourcePath)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, map[string]any{
		configScopeTools: map[string]any{
			"test-tool": map[string]any{
				"name":    "weave",
				"timeout": float64(30),
				"enabled": true,
				"nested": map[string]any{
					removedSandboxModeKey: "fast",
				},
				"limit": float64(5),
			},
		},
	}, got)

	require.NoError(t, cfg.ExtensionConfig(configScopeTools, "test-tool", &target))

	dataAfterSecondCall, err := os.ReadFile(sourcePath)
	require.NoError(t, err)
	assert.Equal(t, string(data), string(dataAfterSecondCall))

	writeFile(t, projectDir, ".weave/settings.json", `{"tools":{"test-tool":{"name":"custom"}}}`)
	require.NoError(t, cfg.ExtensionConfig(configScopeTools, "test-tool", &target))

	dataAfterCustomValue, err := os.ReadFile(sourcePath)
	require.NoError(t, err)

	var customGot map[string]any
	require.NoError(t, json.Unmarshal(dataAfterCustomValue, &customGot))
	assert.Equal(t, "custom", customGot[configScopeTools].(map[string]any)["test-tool"].(map[string]any)["name"])
	assert.InEpsilon(t, float64(30), customGot[configScopeTools].(map[string]any)["test-tool"].(map[string]any)["timeout"], 0)
}
