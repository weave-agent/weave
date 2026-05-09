//go:build linux

package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildBwrapArgs_ReadOnlyRoot(t *testing.T) {
	args := buildBwrapArgs(SandboxConfig{Network: true}, "/tmp/project")

	found := false
	for i := 0; i < len(args)-2; i++ {
		if args[i] == "--ro-bind" && args[i+1] == "/" && args[i+2] == "/" {
			found = true
			break
		}
	}
	assert.True(t, found, "should have --ro-bind / /")
}

func TestBuildBwrapArgs_WritablePaths_DefaultToDir(t *testing.T) {
	args := buildBwrapArgs(SandboxConfig{Network: true}, "/tmp/project")

	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "--bind /tmp/project /tmp/project")
}

func TestBuildBwrapArgs_WritablePaths_Custom(t *testing.T) {
	cfg := SandboxConfig{
		Writable: []string{"/tmp/project", "/tmp/cache"},
		Network:  true,
	}
	args := buildBwrapArgs(cfg, "/tmp/project")

	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "--bind /tmp/project /tmp/project")
	assert.Contains(t, joined, "--bind /tmp/cache /tmp/cache")
}

func TestBuildBwrapArgs_WritablePaths_DotMeansDir(t *testing.T) {
	cfg := SandboxConfig{
		Writable: []string{"."},
		Network:  true,
	}
	args := buildBwrapArgs(cfg, "/tmp/project")

	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "--bind /tmp/project /tmp/project")
}

func TestBuildBwrapArgs_MandatoryDenyDirs(t *testing.T) {
	home, _ := os.UserHomeDir()
	args := buildBwrapArgs(SandboxConfig{Network: true}, "/tmp/project")
	joined := strings.Join(args, " ")

	assert.Contains(t, joined, "--tmpfs "+filepath.Join(home, ".ssh"), "~/.ssh should use --tmpfs")
	assert.Contains(t, joined, "--tmpfs /tmp/project/.git/hooks", ".git/hooks should use --tmpfs")
	assert.Contains(t, joined, "--tmpfs /tmp/project/.weave", ".weave should use --tmpfs")
}

func TestBuildBwrapArgs_MandatoryDenyFiles(t *testing.T) {
	home, _ := os.UserHomeDir()
	args := buildBwrapArgs(SandboxConfig{Network: true}, "/tmp/project")
	joined := strings.Join(args, " ")

	assert.Contains(t, joined, "--ro-bind-try /dev/null "+filepath.Join(home, ".bashrc"))
	assert.Contains(t, joined, "--ro-bind-try /dev/null "+filepath.Join(home, ".zshrc"))
	assert.Contains(t, joined, "--ro-bind-try /dev/null "+filepath.Join(home, ".profile"))
	assert.Contains(t, joined, "--ro-bind-try /dev/null "+filepath.Join(home, ".gitconfig"))
	assert.Contains(t, joined, "--ro-bind-try /dev/null /tmp/project/.git/config")
}

func TestBuildBwrapArgs_UserDenyWriteDirs(t *testing.T) {
	cfg := SandboxConfig{
		DenyWrite: []string{"/secret/dir/"},
		Network:   true,
	}
	args := buildBwrapArgs(cfg, "/tmp/project")
	joined := strings.Join(args, " ")

	assert.Contains(t, joined, "--tmpfs /secret/dir")
}

func TestBuildBwrapArgs_UserDenyWriteFiles(t *testing.T) {
	cfg := SandboxConfig{
		DenyWrite: []string{"/secret/file.txt"},
		Network:   true,
	}
	args := buildBwrapArgs(cfg, "/tmp/project")
	joined := strings.Join(args, " ")

	assert.Contains(t, joined, "--ro-bind-try /dev/null /secret/file.txt")
}

func TestBuildBwrapArgs_PIDIsolation(t *testing.T) {
	args := buildBwrapArgs(SandboxConfig{Network: true}, "/tmp/project")
	joined := strings.Join(args, " ")

	assert.Contains(t, joined, "--unshare-pid")
	assert.Contains(t, joined, "--proc /proc")
}

func TestBuildBwrapArgs_NetworkAllow(t *testing.T) {
	args := buildBwrapArgs(SandboxConfig{Network: true}, "/tmp/project")
	joined := strings.Join(args, " ")

	assert.NotContains(t, joined, "--unshare-net")
}

func TestBuildBwrapArgs_NetworkDeny(t *testing.T) {
	args := buildBwrapArgs(SandboxConfig{Network: false}, "/tmp/project")
	joined := strings.Join(args, " ")

	assert.Contains(t, joined, "--unshare-net")
}

func TestWrapCommandLinux_NoBwrap(t *testing.T) {
	original := os.Getenv("PATH")
	t.Setenv("PATH", "/nonexistent")
	defer t.Setenv("PATH", original)

	_, err := wrapCommandLinux("echo hello", "/tmp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bubblewrap not installed")
}

func TestWrapCommandLinux_WithBwrap(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bwrap not installed")
	}

	s := &Sandbox{cfg: SandboxConfig{Mode: sdk.SandboxAuto, Network: true}}
	sdk.SetSandboxer(s)
	defer sdk.SetSandboxer(nil)

	wrapped, err := wrapCommandLinux("echo hello", "/tmp/project")
	require.NoError(t, err)
	assert.Contains(t, wrapped, "bwrap")
	assert.Contains(t, wrapped, "echo hello")
}

func TestExpandDenyPath_HomePrefix(t *testing.T) {
	result := expandDenyPath("~/.ssh/", "/home/user", "/tmp/project")
	assert.Equal(t, filepath.Join("/home/user", ".ssh"), result)
}

func TestExpandDenyPath_RelativePath(t *testing.T) {
	result := expandDenyPath(".git/config", "/home/user", "/tmp/project")
	assert.Equal(t, "/tmp/project/.git/config", result)
}

func TestExpandDenyPath_AbsolutePath(t *testing.T) {
	result := expandDenyPath("/etc/secret", "/home/user", "/tmp/project")
	assert.Equal(t, "/etc/secret", result)
}
