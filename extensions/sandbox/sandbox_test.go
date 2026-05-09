package sandbox

import (
	"os"
	"testing"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubBus captures handlers registered via On.
type stubBus struct {
	handlers map[string]sdk.Handler
}

func newStubBus() *stubBus {
	return &stubBus{handlers: make(map[string]sdk.Handler)}
}

func (b *stubBus) Publish(ev sdk.Event)           {}
func (b *stubBus) On(topic string, h sdk.Handler) { b.handlers[topic] = h }
func (b *stubBus) OnAll(h sdk.Handler)            {}
func (b *stubBus) Off(h sdk.Handler)              {}
func (b *stubBus) Close() error                   { return nil }

func TestNewSandbox_DefaultConfig(t *testing.T) {
	s, err := NewSandbox(nil)
	require.NoError(t, err)

	defer sdk.SetSandboxer(nil)

	require.NotNil(t, s)
	assert.Equal(t, "sandbox", s.Name())
	assert.Equal(t, ModeAuto, s.Mode())

	assert.Equal(t, s, sdk.GetSandboxer())
}

func TestNewSandbox_SetsGlobalSandboxer(t *testing.T) {
	defer sdk.SetSandboxer(nil)

	s, err := NewSandbox(nil)
	require.NoError(t, err)

	got := sdk.GetSandboxer()
	require.NotNil(t, got)
	assert.Equal(t, s, got)
}

func TestSandbox_ModeOff_WrapCommand(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{Mode: ModeOff}}

	wrapped, err := s.WrapCommand("ls -la", "/tmp")
	require.NoError(t, err)
	assert.Equal(t, "ls -la", wrapped)
}

func TestSandbox_ModeOff_AllowWrite(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{Mode: ModeOff}}

	assert.True(t, s.AllowWrite("/any/path"))
	assert.True(t, s.AllowWrite("~/.ssh/config"))
}

func TestSandbox_ModeOff_AllowRead(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{Mode: ModeOff}}

	assert.True(t, s.AllowRead("/any/path"))
	assert.True(t, s.AllowRead("~/.ssh/id_rsa"))
}

func TestSandbox_ModeAuto_AllowWrite_MandatoryDeny(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{
		Mode:     ModeAuto,
		Writable: []string{"."},
	}}

	home := homeDir(t)

	assert.False(t, s.AllowWrite(home+"/.bashrc"), "should deny .bashrc")
	assert.False(t, s.AllowWrite(home+"/.ssh/something"), "should deny .ssh/")
	assert.False(t, s.AllowWrite(home+"/.zshrc"), "should deny .zshrc")
	assert.False(t, s.AllowWrite(home+"/.profile"), "should deny .profile")
	assert.False(t, s.AllowWrite(home+"/.gitconfig"), "should deny .gitconfig")
}

func TestSandbox_ModeAuto_AllowRead_MandatoryDeny(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{
		Mode:     ModeAuto,
		Writable: []string{"."},
	}}

	home := homeDir(t)

	assert.False(t, s.AllowRead(home+"/.ssh/id_rsa"), "should deny ssh key")
	assert.False(t, s.AllowRead(home+"/.ssh/id_ed25519"), "should deny ssh key")
	assert.False(t, s.AllowRead(home+"/.aws/credentials"), "should deny aws creds")
	assert.False(t, s.AllowRead("/project/.env"), "should deny .env")
	assert.False(t, s.AllowRead("/project/.env.local"), "should deny .env.*")
}

func TestSandbox_ModeAuto_AllowWrite_WritablePaths(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{
		Mode:     ModeAuto,
		Writable: []string{"/project"},
	}}

	assert.True(t, s.AllowWrite("/project/file.go"))
	assert.True(t, s.AllowWrite("/project/sub/file.go"))
	assert.False(t, s.AllowWrite("/other/file.go"))
}

func TestSandbox_ModeAuto_AllowWrite_DenyWriteOverrides(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{
		Mode:      ModeAuto,
		Writable:  []string{"/project"},
		DenyWrite: []string{"/project/secret"},
	}}

	assert.False(t, s.AllowWrite("/project/secret/key"))
	assert.True(t, s.AllowWrite("/project/other"))
}

func TestSandbox_ModeAuto_AllowRead_DenyReadOverrides(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{
		Mode:     ModeAuto,
		DenyRead: []string{"/project/secrets"},
	}}

	assert.False(t, s.AllowRead("/project/secrets/key.pem"))
	assert.True(t, s.AllowRead("/project/main.go"))
}

func TestSandbox_ModeAuto_AllowWrite_NoWritableConfig(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{
		Mode:     ModeAuto,
		Writable: nil,
	}}

	assert.True(t, s.AllowWrite("/tmp/file"))
}

func TestSandbox_Subscribe_ModeChange(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{Mode: ModeAuto}}
	bus := newStubBus()

	err := s.Subscribe(bus)
	require.NoError(t, err)

	handler, ok := bus.handlers["sandbox.mode.change"]
	require.True(t, ok, "expected handler for sandbox.mode.change")

	handler(sdk.NewEvent("sandbox.mode.change", ModeReadonly))
	assert.Equal(t, ModeReadonly, s.Mode())

	handler(sdk.NewEvent("sandbox.mode.change", ModeOff))
	assert.Equal(t, ModeOff, s.Mode())
}

func TestSandbox_Subscribe_InvalidPayload(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{Mode: ModeAuto}}
	bus := newStubBus()

	err := s.Subscribe(bus)
	require.NoError(t, err)

	handler, ok := bus.handlers["sandbox.mode.change"]
	require.True(t, ok)

	handler(sdk.NewEvent("sandbox.mode.change", 42))
	assert.Equal(t, ModeAuto, s.Mode())
}

func TestSandbox_Close(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{Mode: ModeAuto}}
	assert.NoError(t, s.Close())
}

func TestSandbox_SetMode(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{Mode: ModeAuto}}

	s.SetMode(ModeReadonly)
	assert.Equal(t, ModeReadonly, s.Mode())

	s.SetMode(ModeAsk)
	assert.Equal(t, ModeAsk, s.Mode())
}

func TestPathMatches(t *testing.T) {
	tests := []struct {
		path    string
		pattern string
		want    bool
	}{
		{"/project/file.go", "/project", true},
		{"/project/sub/file.go", "/project", true},
		{"/other/file.go", "/project", false},
		{"/exact/path", "/exact/path", true},
	}

	for _, tt := range tests {
		got := pathMatches(tt.path, tt.pattern)
		assert.Equal(t, tt.want, got, "pathMatches(%q, %q)", tt.path, tt.pattern)
	}
}

func TestIsDeniedWrite(t *testing.T) {
	home := homeDir(t)

	tests := []struct {
		path string
		want bool
	}{
		{home + "/.bashrc", true},
		{home + "/.ssh/known_hosts", true},
		{home + "/.zshrc", true},
		{home + "/.profile", true},
		{home + "/.gitconfig", true},
		{"/tmp/file.go", false},
		{"/project/main.go", false},
	}

	for _, tt := range tests {
		got := isDeniedWrite(tt.path)
		assert.Equal(t, tt.want, got, "isDeniedWrite(%q)", tt.path)
	}
}

func TestIsDeniedRead(t *testing.T) {
	home := homeDir(t)

	tests := []struct {
		path string
		want bool
	}{
		{home + "/.ssh/id_rsa", true},
		{home + "/.ssh/id_ed25519", true},
		{home + "/.aws/credentials", true},
		{"/project/.env", true},
		{"/project/.env.production", true},
		{"/project/main.go", false},
		{"/etc/hosts", false},
	}

	for _, tt := range tests {
		got := isDeniedRead(tt.path)
		assert.Equal(t, tt.want, got, "isDeniedRead(%q)", tt.path)
	}
}

func homeDir(t *testing.T) string {
	t.Helper()

	dir, err := os.UserHomeDir()
	require.NoError(t, err)

	return dir
}
