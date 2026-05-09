//go:build darwin

package sandbox

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSeatbeltProfile_Version1(t *testing.T) {
	profile := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")
	require.True(t, strings.HasPrefix(profile, "(version 1)"), "profile must start with version 1")
}

func TestGenerateSeatbeltProfile_DenyDefault(t *testing.T) {
	profile := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")
	assert.Contains(t, profile, "(deny default)\n")
}

func TestGenerateSeatbeltProfile_AllowFileRead(t *testing.T) {
	profile := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")
	assert.Contains(t, profile, "(allow file-read*)\n")
}

func TestGenerateSeatbeltProfile_ReadDenySSHKeys(t *testing.T) {
	home, _ := os.UserHomeDir()
	sshDir := filepath.Join(home, ".ssh")

	profile := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")

	assert.Contains(t, profile, `(deny file-read* (regex #"^`+regexp.QuoteMeta(sshDir)+`/id_.*$"))`,
		"must deny reading ssh private keys via regex")
}

func TestGenerateSeatbeltProfile_ReadDenyAWSCredentials(t *testing.T) {
	home, _ := os.UserHomeDir()
	awsCreds := filepath.Join(home, ".aws", "credentials")

	profile := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")

	assert.Contains(t, profile, `(deny file-read* (literal "`+awsCreds+`"))`,
		"must deny reading AWS credentials")
}

func TestGenerateSeatbeltProfile_ReadDenyEnvFiles(t *testing.T) {
	profile := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")

	assert.Contains(t, profile, `regex #"/\\.env$"`, "must deny reading .env files")
	assert.Contains(t, profile, `regex #"/\\.env\\.[^/]+$"`, "must deny reading .env.* files")
}

func TestGenerateSeatbeltProfile_WritablePaths(t *testing.T) {
	cfg := SandboxConfig{
		Writable: []string{"/tmp/project", "/tmp/cache"},
		Network:  true,
	}
	profile := generateSeatbeltProfile(cfg, "/tmp/project")

	assert.Contains(t, profile, `(allow file-write* (subpath "/tmp/project"))`)
	assert.Contains(t, profile, `(allow file-write* (subpath "/tmp/cache"))`)
}

func TestGenerateSeatbeltProfile_WritablePaths_DefaultToDir(t *testing.T) {
	cfg := SandboxConfig{
		Writable: nil,
		Network:  true,
	}
	profile := generateSeatbeltProfile(cfg, "/tmp/project")

	assert.Contains(t, profile, `(allow file-write* (subpath "/tmp/project"))`)
}

func TestGenerateSeatbeltProfile_WritablePaths_DotMeansCWD(t *testing.T) {
	cfg := SandboxConfig{
		Writable: []string{"."},
		Network:  true,
	}
	profile := generateSeatbeltProfile(cfg, "/tmp/project")

	assert.Contains(t, profile, `(allow file-write* (subpath "/tmp/project"))`)
}

func TestGenerateSeatbeltProfile_WriteDenySSHDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	sshDir := filepath.Join(home, ".ssh")

	profile := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")

	assert.Contains(t, profile, `(deny file-write* (subpath "`+sshDir+`"))`)
}

func TestGenerateSeatbeltProfile_WriteDenyShellFiles(t *testing.T) {
	home, _ := os.UserHomeDir()
	profile := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")

	assert.Contains(t, profile, `(deny file-write* (literal "`+filepath.Join(home, ".bashrc")+`"))`)
	assert.Contains(t, profile, `(deny file-write* (literal "`+filepath.Join(home, ".zshrc")+`"))`)
	assert.Contains(t, profile, `(deny file-write* (literal "`+filepath.Join(home, ".profile")+`"))`)
	assert.Contains(t, profile, `(deny file-write* (literal "`+filepath.Join(home, ".gitconfig")+`"))`)
}

func TestGenerateSeatbeltProfile_WriteDenyGitDir(t *testing.T) {
	profile := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")

	assert.Contains(t, profile, `(deny file-write* (subpath "/tmp/project/.git/hooks"))`)
	assert.Contains(t, profile, `(deny file-write* (literal "/tmp/project/.git/config"))`)
}

func TestGenerateSeatbeltProfile_WriteDenyWeaveDir(t *testing.T) {
	profile := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")

	assert.Contains(t, profile, `(deny file-write* (subpath "/tmp/project/.weave"))`)
}

func TestGenerateSeatbeltProfile_ProcessRules(t *testing.T) {
	profile := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")

	assert.Contains(t, profile, "(allow process-exec)\n")
	assert.Contains(t, profile, "(allow process-fork)\n")
}

func TestGenerateSeatbeltProfile_NetworkAllow(t *testing.T) {
	profile := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")

	assert.Contains(t, profile, "(allow network*)\n")
	assert.NotContains(t, profile, "(deny network*)")
}

func TestGenerateSeatbeltProfile_NetworkDeny(t *testing.T) {
	profile := generateSeatbeltProfile(SandboxConfig{Network: false}, "/tmp/project")

	assert.Contains(t, profile, "(deny network*)\n")
	assert.NotContains(t, profile, "(allow network*)")
}

func TestWrapCommandDarwin_NoSandboxExec(t *testing.T) {
	original := os.Getenv("PATH")

	t.Setenv("PATH", "/nonexistent")
	defer t.Setenv("PATH", original)

	_, err := wrapCommandDarwinWithConfig("ls -la", "/tmp", SandboxConfig{Network: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sandbox-exec not found")
}

func TestWrapCommandDarwin_WithSandboxExec(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin only")
	}

	wrapped, err := wrapCommandDarwinWithConfig("echo hello", "/tmp/project", SandboxConfig{Network: true})
	require.NoError(t, err)
	assert.Contains(t, wrapped, "sandbox-exec -p '")
	assert.Contains(t, wrapped, "echo hello")
}
