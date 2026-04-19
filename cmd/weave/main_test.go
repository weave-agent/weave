package main

import (
	"os"
	"strings"
	"testing"

	"weave/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeUnique(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"empty", nil, []string{}},
		{"single", []string{"a"}, []string{"a"}},
		{"no dupes", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"removes dupes", []string{"a", "b", "a", "c", "b"}, []string{"a", "b", "c"}},
		{"all same", []string{"x", "x", "x"}, []string{"x"}},
		{"preserves order", []string{"loop", "anthropic", "bash"}, []string{"loop", "anthropic", "bash"}},
		{"core plus optional overlap", []string{"loop", "anthropic", "anthropic"}, []string{"loop", "anthropic"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeUnique(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRunFlagParsing(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
	}{
		{"invalid flag", []string{"-xyz"}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantCode, run(tt.args...))
		})
	}
}

func TestRunMissingConfig(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))

	defer func() { _ = os.Chdir(origWd) }()

	assert.Equal(t, 1, run())
}

func TestRunExtensionOverride(t *testing.T) {
	dir := t.TempDir()

	cfgFile := dir + "/.weave.yaml"
	require.NoError(t, os.WriteFile(cfgFile, []byte("extensions: [noop]\n"), 0o600))

	origWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))

	defer func() { _ = os.Chdir(origWd) }()

	assert.Equal(t, 1, run("-e", "ext1,ext2"))
}

func TestRunCoreDefaultsUsed(t *testing.T) {
	dir := t.TempDir()

	cfgFile := dir + "/.weave.yaml"
	require.NoError(t, os.WriteFile(cfgFile, []byte("{}\n"), 0o600))

	require.NoError(t, os.WriteFile(dir+"/go.mod", []byte("module weave\n\ngo 1.24\n"), 0o600))

	origWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))

	defer func() { _ = os.Chdir(origWd) }()

	old := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stderr = w

	defer func() { os.Stderr = old }()

	exitCode := run()

	_ = w.Close()

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	_ = r.Close()

	stderr := string(buf[:n])

	assert.Equal(t, 1, exitCode)
	assert.True(t, strings.Contains(stderr, "loop") || strings.Contains(stderr, "anthropic"),
		"stderr should mention 'loop' or 'anthropic' (core defaults), got: %q", stderr)
}

func TestValidateCoreConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *config.File
		wantErr error
	}{
		{
			"valid defaults",
			&config.File{Core: config.CoreConfig{AgentLoop: "loop", Providers: []string{"anthropic"}}},
			nil,
		},
		{
			"empty agent_loop",
			&config.File{Core: config.CoreConfig{AgentLoop: "", Providers: []string{"anthropic"}}},
			errAgentLoopRequired,
		},
		{
			"no providers",
			&config.File{Core: config.CoreConfig{AgentLoop: "loop", Providers: nil}},
			errProviderRequired,
		},
		{
			"empty providers",
			&config.File{Core: config.CoreConfig{AgentLoop: "loop", Providers: []string{}}},
			errProviderRequired,
		},
		{
			"duplicate providers",
			&config.File{Core: config.CoreConfig{AgentLoop: "loop", Providers: []string{"anthropic", "anthropic"}}},
			errDuplicateProvider,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCoreConfig(tt.config)
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr.Error())
			}
		})
	}
}
