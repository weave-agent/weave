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

	launcherPath, _ := os.Executable()
	origArgsJSON, err := json.Marshal(os.Args)
	require.NoError(t, err)

	env := append(
		os.Environ(),
		"WEAVE_LAUNCHER_PATH="+launcherPath,
		"WEAVE_BUILD_HASH="+hash,
		"WEAVE_ORIG_ARGS="+string(origArgsJSON),
	)

	cmd := exec.Command(binPath)
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	require.Len(t, lines, 3)

	assert.Equal(t, "LAUNCHER="+launcherPath, lines[0])
	assert.Equal(t, "HASH="+hash, lines[1])

	var parsedArgs []string
	require.NoError(t, json.Unmarshal(origArgsJSON, &parsedArgs))
	assert.Equal(t, []string{"weave", "-p", "test prompt"}, parsedArgs)
}
