//go:build darwin

package sandbox

import (
	"fmt"
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
	profile, _, err := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(profile, "(version 1)"), "profile must start with version 1")
}

func TestGenerateSeatbeltProfile_DenyDefault(t *testing.T) {
	profile, _, err := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")
	require.NoError(t, err)
	assert.Contains(t, profile, "(deny default)")
}

func TestGenerateSeatbeltProfile_ProcessRules(t *testing.T) {
	profile, _, err := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")
	require.NoError(t, err)
	assert.Contains(t, profile, "(allow process-exec)")
	assert.Contains(t, profile, "(allow process-fork)")
	assert.Contains(t, profile, "(allow signal (target same-sandbox))")
}

func TestGenerateSeatbeltProfile_SysctlRead(t *testing.T) {
	profile, _, err := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")
	require.NoError(t, err)
	assert.Contains(t, profile, "(allow sysctl-read")
	assert.Contains(t, profile, "hw.pagesize")
}

func TestGenerateSeatbeltProfile_DevNull(t *testing.T) {
	profile, _, err := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")
	require.NoError(t, err)
	assert.Contains(t, profile, "/dev/null")
	assert.Contains(t, profile, "CHARACTER-DEVICE")
}

func TestGenerateSeatbeltProfile_MachLookup(t *testing.T) {
	profile, _, err := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")
	require.NoError(t, err)
	assert.Contains(t, profile, "com.apple.system.opendirectoryd.libinfo")
}

func TestGenerateSeatbeltProfile_PlatformDefaults(t *testing.T) {
	profile, _, err := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")
	require.NoError(t, err)
	assert.Contains(t, profile, "(subpath \"/tmp\")")
	assert.Contains(t, profile, "(subpath \"/private/tmp\")")
	assert.Contains(t, profile, "/usr/lib")
	assert.Contains(t, profile, "/System/Library/Frameworks")
}

func TestGenerateSeatbeltProfile_ReadDenySSHKeys(t *testing.T) {
	home, _ := os.UserHomeDir()
	sshDir := filepath.Join(home, ".ssh")

	profile, _, err := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")
	require.NoError(t, err)

	assert.Contains(t, profile, `(deny file-read* (regex #"^`+regexp.QuoteMeta(sshDir)+`/id_.*$"))`,
		"must deny reading ssh private keys via regex")
}

func TestGenerateSeatbeltProfile_ReadDenyAWSCredentials(t *testing.T) {
	home, _ := os.UserHomeDir()
	awsCreds := filepath.Join(home, ".aws", "credentials")

	profile, _, err := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")
	require.NoError(t, err)

	assert.Contains(t, profile, `(deny file-read* (literal "`+awsCreds+`"))`,
		"must deny reading AWS credentials")
}

func TestGenerateSeatbeltProfile_ReadDenyEnvFiles(t *testing.T) {
	profile, _, err := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")
	require.NoError(t, err)

	assert.Contains(t, profile, `regex #"/\.env$"`, "must deny reading .env files")
	assert.Contains(t, profile, `regex #"/\.env\.[^/]+$"`, "must deny reading .env.* files")
}

func TestGenerateSeatbeltProfile_WritablePaths_WithParams(t *testing.T) {
	cfg := SandboxConfig{
		Writable: []string{"/tmp/project", "/tmp/cache"},
		Network:  true,
	}
	profile, params, err := generateSeatbeltProfile(cfg, "/tmp/project")
	require.NoError(t, err)

	assert.Contains(t, profile, `WRITABLE_ROOT_0`)
	assert.Contains(t, profile, `WRITABLE_ROOT_1`)
	assert.Contains(t, params, `WRITABLE_ROOT_0=/tmp/project`)
	assert.Contains(t, params, `WRITABLE_ROOT_1=/tmp/cache`)
}

func TestGenerateSeatbeltProfile_WritablePaths_DefaultToDir(t *testing.T) {
	cfg := SandboxConfig{
		Writable: nil,
		Network:  true,
	}
	profile, params, err := generateSeatbeltProfile(cfg, "/tmp/project")
	require.NoError(t, err)

	assert.Contains(t, profile, `WRITABLE_ROOT_0`)
	assert.Contains(t, params, `WRITABLE_ROOT_0=/tmp/project`)
}

func TestGenerateSeatbeltProfile_WritablePaths_DotMeansCWD(t *testing.T) {
	cfg := SandboxConfig{
		Writable: []string{"."},
		Network:  true,
	}
	profile, params, err := generateSeatbeltProfile(cfg, "/tmp/project")
	require.NoError(t, err)

	assert.Contains(t, profile, `WRITABLE_ROOT_0`)
	assert.Contains(t, params, `WRITABLE_ROOT_0=/tmp/project`)
}

func TestGenerateSeatbeltProfile_WriteDenySSHDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	sshDir := filepath.Join(home, ".ssh")

	cfg := SandboxConfig{Network: true}
	profile, params, err := generateSeatbeltProfile(cfg, "/tmp/project")
	require.NoError(t, err)

	assert.Contains(t, profile, `WRITABLE_DENY_0`)
	found := false
	for _, p := range params {
		if strings.HasPrefix(p, "WRITABLE_DENY_0=") && strings.Contains(p, sshDir) {
			found = true
			break
		}
	}
	assert.True(t, found, "SSH dir should be in deny params")
	assert.Contains(t, profile, `(require-not (subpath (param "WRITABLE_DENY_0")))`)
}

func TestGenerateSeatbeltProfile_WriteDenyShellFiles(t *testing.T) {
	home, _ := os.UserHomeDir()
	cfg := SandboxConfig{Network: true}
	profile, params, err := generateSeatbeltProfile(cfg, "/tmp/project")
	require.NoError(t, err)

	assert.Contains(t, profile, `WRITABLE_DENY_1`)
	assert.Contains(t, profile, `WRITABLE_DENY_2`)
	assert.Contains(t, profile, `WRITABLE_DENY_3`)
	assert.Contains(t, profile, `WRITABLE_DENY_4`)

	bashrcParam := fmt.Sprintf("WRITABLE_DENY_1=%s", filepath.Join(home, ".bashrc"))
	assert.Contains(t, params, bashrcParam)
}

func TestGenerateSeatbeltProfile_UserDenyWriteDirs(t *testing.T) {
	cfg := SandboxConfig{
		DenyWrite: []string{"/secret/dir/"},
		Network:   true,
	}
	profile, params, err := generateSeatbeltProfile(cfg, "/tmp/project")
	require.NoError(t, err)

	offset := len(mandatoryDenyWritePaths)
	paramKey := fmt.Sprintf("WRITABLE_DENY_%d", offset)
	assert.Contains(t, profile, paramKey)
	assert.Contains(t, params, paramKey+"=/secret/dir/")
	assert.Contains(t, profile, fmt.Sprintf(`(require-not (subpath (param "%s")))`, paramKey))
}

func TestGenerateSeatbeltProfile_UserDenyWriteFiles(t *testing.T) {
	cfg := SandboxConfig{
		DenyWrite: []string{"/secret/file.txt"},
		Network:   true,
	}
	profile, params, err := generateSeatbeltProfile(cfg, "/tmp/project")
	require.NoError(t, err)

	offset := len(mandatoryDenyWritePaths)
	paramKey := fmt.Sprintf("WRITABLE_DENY_%d", offset)
	assert.Contains(t, profile, paramKey)
	assert.Contains(t, params, paramKey+"=/secret/file.txt")
	assert.Contains(t, profile, fmt.Sprintf(`(require-not (literal (param "%s")))`, paramKey))
}

func TestGenerateSeatbeltProfile_UserDenyReadDirs(t *testing.T) {
	cfg := SandboxConfig{
		DenyRead: []string{"/private/docs/"},
		Network:  true,
	}
	profile, _, err := generateSeatbeltProfile(cfg, "/tmp/project")
	require.NoError(t, err)

	assert.Contains(t, profile, `(deny file-read* (subpath "/private/docs"))`)
}

func TestGenerateSeatbeltProfile_UserDenyReadFiles(t *testing.T) {
	cfg := SandboxConfig{
		DenyRead: []string{"/private/secret.key"},
		Network:  true,
	}
	profile, _, err := generateSeatbeltProfile(cfg, "/tmp/project")
	require.NoError(t, err)

	assert.Contains(t, profile, `(deny file-read* (literal "/private/secret.key"))`)
}

func TestGenerateSeatbeltProfile_NetworkAllow(t *testing.T) {
	profile, _, err := generateSeatbeltProfile(SandboxConfig{Network: true}, "/tmp/project")
	require.NoError(t, err)
	assert.Contains(t, profile, "(allow network-outbound)")
	assert.Contains(t, profile, "(allow network-inbound)")
	assert.Contains(t, profile, "com.apple.SecurityServer")
}

func TestGenerateSeatbeltProfile_NetworkDeny(t *testing.T) {
	profile, _, err := generateSeatbeltProfile(SandboxConfig{Network: false}, "/tmp/project")
	require.NoError(t, err)
	assert.Contains(t, profile, "(deny network*)")
	assert.NotContains(t, profile, "(allow network-outbound)")
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
	assert.Contains(t, wrapped, "sandbox-exec ")
	assert.Contains(t, wrapped, "echo hello")
	assert.Contains(t, wrapped, "-p '")
	assert.Contains(t, wrapped, "-DWRITABLE_ROOT_0=")
}

func TestWrapCommandDarwin_ReadonlyMode(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin only")
	}

	cfg := SandboxConfig{
		Writable:  []string{""},
		Network:   true,
		DenyWrite: []string{},
		DenyRead:  []string{},
	}
	wrapped, err := wrapCommandDarwinWithConfig("echo hello", "/tmp/project", cfg)
	require.NoError(t, err)
	assert.Contains(t, wrapped, "sandbox-exec ")
	assert.NotContains(t, wrapped, "WRITABLE_ROOT")
}

