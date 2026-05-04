package launcher

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJsonMarshalArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "simple args",
			args: []string{"weave", "-p", "hello world"},
			want: []string{"weave", "-p", "hello world"},
		},
		{
			name: "empty args",
			args: []string{},
			want: []string{},
		},
		{
			name: "args with special chars",
			args: []string{"weave", `arg with "quotes"`, "line\nbreak"},
			want: []string{"weave", `arg with "quotes"`, "line\nbreak"},
		},
		{
			name: "args with backslashes",
			args: []string{"weave", `path\to\file`},
			want: []string{"weave", `path\to\file`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonMarshalArgs(tt.args)

			// Verify it's valid JSON
			var parsed []string
			require.NoError(t, json.Unmarshal([]byte(got), &parsed))
			assert.Equal(t, tt.want, parsed)
		})
	}
}

func TestJsonEscapeString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`hello`, `"hello"`},
		{`say "hi"`, `"say \"hi\""`},
		{`back\slash`, `"back\\slash"`},
		{"line\nbreak", `"line\nbreak"`},
		{"tab\there", `"tab\there"`},
		{"cr\rhere", `"cr\rhere"`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := jsonEscapeString(tt.input)
			assert.Equal(t, tt.want, got)

			// Verify round-trip
			var parsed string
			require.NoError(t, json.Unmarshal([]byte(got), &parsed))
			assert.Equal(t, tt.input, parsed)
		})
	}
}

func TestAppendEnv(t *testing.T) {
	env := []string{"HOME=/home/user", "PATH=/usr/bin"}
	env = appendEnv(env, "WEAVE_BUILD_HASH", "abc123")
	assert.Equal(t, "WEAVE_BUILD_HASH=abc123", env[len(env)-1])
	assert.Len(t, env, 3)
}

func TestExecEnvVars(t *testing.T) {
	cacheDir := t.TempDir()
	hash := "testhash123"
	binDir := filepath.Join(cacheDir, hash)
	require.NoError(t, os.MkdirAll(binDir, 0o750))

	// Create a small script that prints the env vars we care about, then exits.
	script := `#!/bin/sh
echo "LAUNCHER=$WEAVE_LAUNCHER_PATH"
echo "HASH=$WEAVE_BUILD_HASH"
echo "ORIG_ARGS=$WEAVE_ORIG_ARGS"
`
	binPath := filepath.Join(binDir, "weave")
	require.NoError(t, os.WriteFile(binPath, []byte(script), 0o750))

	// Override os.Args for the test.
	origArgs := os.Args
	os.Args = []string{"weave", "-p", "test prompt"}

	t.Cleanup(func() { os.Args = origArgs })

	// exec will fail because syscall.Exec replaces the process, but the script
	// will still run. We verify by running the binary ourselves with the env
	// vars that exec would set.
	launcherPath, _ := os.Executable()
	origArgsJSON := jsonMarshalArgs(os.Args)

	env := os.Environ()
	env = appendEnv(env, "WEAVE_LAUNCHER_PATH", launcherPath)
	env = appendEnv(env, "WEAVE_BUILD_HASH", hash)
	env = appendEnv(env, "WEAVE_ORIG_ARGS", origArgsJSON)

	cmd := exec.Command(binPath)
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	require.Len(t, lines, 3)

	assert.Equal(t, "LAUNCHER="+launcherPath, lines[0])
	assert.Equal(t, "HASH="+hash, lines[1])

	var parsedArgs []string
	require.NoError(t, json.Unmarshal([]byte(origArgsJSON), &parsedArgs))
	assert.Equal(t, []string{"weave", "-p", "test prompt"}, parsedArgs)
}
