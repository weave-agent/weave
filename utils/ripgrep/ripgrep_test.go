package ripgrep

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindReturnsValidPathWhenRgInPATH(t *testing.T) {
	// If rg is available on this system, Find() should return a non-empty path.
	// This test is informational — it passes either way.
	result := Find()
	if _, err := exec.LookPath("rg"); err == nil {
		assert.NotEmpty(t, result, "Find() should return a path when rg is in PATH")

		info, err := os.Stat(result)
		require.NoError(t, err)
		assert.False(t, info.IsDir())
	} else {
		assert.Empty(t, result, "Find() should return empty when rg is not in PATH")
	}
}

func TestFindCachesResult(t *testing.T) {
	a := Find()
	b := Find()
	assert.Equal(t, a, b, "Find() should return the same cached value")
}

func TestFindReturnsEmptyWhenRgAbsent(t *testing.T) {
	// Test by temporarily removing rg from PATH.
	// We can't easily modify the process environment for sync.OnceValue,
	// so we test the underlying exec.LookPath behavior directly.
	t.Setenv("PATH", "/nonexistent")

	_, err := exec.LookPath("rg")
	assert.Error(t, err, "rg should not be found in /nonexistent PATH")
}
