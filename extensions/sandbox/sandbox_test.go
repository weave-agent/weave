package sandbox

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/weave-agent/weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubBus captures handlers registered via On and events published.
type stubBus struct {
	mu        sync.Mutex
	handlers  map[string]sdk.Handler
	published []sdk.Event
}

func newStubBus() *stubBus {
	return &stubBus{handlers: make(map[string]sdk.Handler)}
}

func (b *stubBus) Publish(ev sdk.Event) {
	b.mu.Lock()
	b.published = append(b.published, ev)
	b.mu.Unlock()

	// Deliver to matching handler if registered (for testing event flow).
	if h, ok := b.handlers[ev.Topic]; ok {
		_ = h(ev)
	}
}

func (b *stubBus) events() []sdk.Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	return append([]sdk.Event(nil), b.published...)
}

func (b *stubBus) On(topic string, h sdk.Handler) { b.handlers[topic] = h }
func (b *stubBus) OnAll(h sdk.Handler)            {}
func (b *stubBus) Off(h sdk.Handler)              {}
func (b *stubBus) Close() error                   { return nil }

func TestNewSandbox_DefaultConfig(t *testing.T) {
	s, err := NewSandbox(nil, SandboxConfig{})
	require.NoError(t, err)

	require.NotNil(t, s)
	assert.Equal(t, "sandbox", s.Name())
	assert.Equal(t, SandboxAuto, s.Mode())
}

func TestSandbox_ModeOff_WrapCommand(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{Mode: SandboxOff}}

	wrapped, err := s.WrapCommand("ls -la", "/tmp")
	require.NoError(t, err)
	assert.Equal(t, "ls -la", wrapped)
}

func TestSandbox_ModeOff_AllowWrite(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{Mode: SandboxOff}}

	assert.True(t, s.AllowWrite("/any/path"))
	assert.True(t, s.AllowWrite("~/.ssh/config"))
}

func TestSandbox_ModeOff_AllowRead(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{Mode: SandboxOff}}

	assert.True(t, s.AllowRead("/any/path"))
	assert.True(t, s.AllowRead("~/.ssh/id_rsa"))
}

func TestSandbox_ModeAuto_AllowWrite_MandatoryDeny(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{
		Mode:     SandboxAuto,
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
		Mode:     SandboxAuto,
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
		Mode:     SandboxAuto,
		Writable: []string{"/project"},
	}}

	assert.True(t, s.AllowWrite("/project/file.go"))
	assert.True(t, s.AllowWrite("/project/sub/file.go"))
	assert.False(t, s.AllowWrite("/other/file.go"))
}

func TestSandbox_ModeAuto_AllowWrite_DenyWriteOverrides(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{
		Mode:      SandboxAuto,
		Writable:  []string{"/project"},
		DenyWrite: []string{"/project/secret"},
	}}

	assert.False(t, s.AllowWrite("/project/secret/key"))
	assert.True(t, s.AllowWrite("/project/other"))
}

func TestSandbox_ModeAuto_AllowRead_DenyReadOverrides(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{
		Mode:     SandboxAuto,
		DenyRead: []string{"/project/secrets"},
	}}

	assert.False(t, s.AllowRead("/project/secrets/key.pem"))
	assert.True(t, s.AllowRead("/project/main.go"))
}

func TestSandbox_ModeAuto_AllowWrite_NoWritableConfig(t *testing.T) {
	cwd, _ := os.Getwd()
	s := &Sandbox{
		cfg: SandboxConfig{Mode: SandboxAuto},
		cwd: cwd,
	}

	assert.True(t, s.AllowWrite(filepath.Join(cwd, "file.go")), "path under CWD should be allowed")
	assert.False(t, s.AllowWrite("/tmp/file"), "path outside CWD should be denied")
}

func TestSandbox_Subscribe_ModeChange(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{Mode: SandboxAuto}}
	bus := newStubBus()

	err := s.Subscribe(bus)
	require.NoError(t, err)

	handler, ok := bus.handlers["sandbox.mode.change"]
	require.True(t, ok, "expected handler for sandbox.mode.change")

	require.NoError(t, handler(sdk.NewEvent("sandbox.mode.change", SandboxReadonly)))
	assert.Equal(t, SandboxReadonly, s.Mode())

	require.NoError(t, handler(sdk.NewEvent("sandbox.mode.change", SandboxOff)))
	assert.Equal(t, SandboxOff, s.Mode())
}

func TestSandbox_Subscribe_InvalidPayload(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{Mode: SandboxAuto}}
	bus := newStubBus()

	err := s.Subscribe(bus)
	require.NoError(t, err)

	handler, ok := bus.handlers["sandbox.mode.change"]
	require.True(t, ok)

	require.NoError(t, handler(sdk.NewEvent("sandbox.mode.change", 42)))
	assert.Equal(t, SandboxAuto, s.Mode())
}

func TestSandbox_Close(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{Mode: SandboxAuto}}
	assert.NoError(t, s.Close())
}

func TestSandbox_SetMode(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{Mode: SandboxAuto}}

	s.SetMode(SandboxReadonly)
	assert.Equal(t, SandboxReadonly, s.Mode())

	s.SetMode(SandboxAsk)
	assert.Equal(t, SandboxAsk, s.Mode())
}

// --- Mode-specific WrapCommand tests ---

func TestWrapCommand_ModeOff_ReturnsUnchanged(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{Mode: SandboxOff}}
	wrapped, err := s.WrapCommand("rm -rf /", "/tmp")
	require.NoError(t, err)
	assert.Equal(t, "rm -rf /", wrapped)
}

func TestWrapCommand_ModeAuto_WrapsPlatform(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{Mode: SandboxAuto, Network: true}}
	wrapped, err := s.WrapCommand("ls -la", "/tmp")
	require.NoError(t, err)
	// On darwin, should be wrapped with sandbox-exec if available
	// The exact output depends on platform, just verify no error
	assert.NotEmpty(t, wrapped)
}

func TestWrapCommand_ModeReadonly_WrapsWithNoWritable(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{
		Mode:     SandboxReadonly,
		Writable: []string{"/project"},
		Network:  true,
	}}

	wrapped, err := s.WrapCommand("echo hello", "/tmp")
	require.NoError(t, err)
	assert.NotEmpty(t, wrapped)
}

func TestWrapCommand_ModeReadonly_BlocksAllWrites(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{
		Mode:     SandboxReadonly,
		Writable: []string{"/project"},
	}}
	assert.False(t, s.AllowWrite("/project/file.go"), "readonly should block all writes")
	assert.False(t, s.AllowWrite("/tmp/file"), "readonly should block all writes")
	assert.False(t, s.AllowWrite("/any/path"), "readonly should block all writes")
}

func TestWrapCommand_ModeReadonly_AllowsRead(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{
		Mode:     SandboxReadonly,
		Writable: []string{"/project"},
	}}
	assert.True(t, s.AllowRead("/project/main.go"))
	assert.True(t, s.AllowRead("/etc/hosts"))
}

func TestWrapCommand_ModeAsk_Headless_Denies(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{Mode: SandboxAsk}, headless: true}
	_, err := s.WrapCommand("rm -rf /", "/tmp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "headless mode")
}

func TestWrapCommand_ModeAsk_NoBus_Denies(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{Mode: SandboxAsk}, headless: false}
	_, err := s.WrapCommand("echo hello", "/tmp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bus not available")
}

func TestWrapCommand_ModeAsk_Approved(t *testing.T) {
	bus := newStubBus()
	s := &Sandbox{cfg: SandboxConfig{Mode: SandboxAsk}, headless: false}
	s.bus = bus

	// Use Subscribe to register approval handlers on the bus.
	require.NoError(t, s.Subscribe(bus))

	// Start WrapCommand in background — it will block waiting for approval.
	done := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		wrapped, err := s.WrapCommand("echo hello", "/tmp")
		if err != nil {
			errCh <- err
			return
		}

		done <- wrapped
	}()

	// Wait for the approve event to be published by the stubBus delivering
	// to the sandbox.approve handler (which doesn't exist on bus yet, so
	// we need to wait for the goroutine to publish manually).
	require.Eventually(t, func() bool {
		for _, ev := range bus.events() {
			if ev.Topic == "sandbox.approve" {
				return true
			}
		}

		return false
	}, 2*time.Second, 50*time.Millisecond, "expected sandbox.approve event")

	// Simulate TUI publishing an approved event.
	bus.Publish(sdk.NewEvent("sandbox.approved", map[string]string{"command": "echo hello"}))

	select {
	case wrapped := <-done:
		assert.Equal(t, "echo hello", wrapped)
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for WrapCommand to complete")
	}
}

func TestWrapCommand_ModeAsk_Denied(t *testing.T) {
	bus := newStubBus()
	s := &Sandbox{cfg: SandboxConfig{Mode: SandboxAsk}, headless: false}
	s.bus = bus

	require.NoError(t, s.Subscribe(bus))

	done := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		wrapped, err := s.WrapCommand("rm -rf /", "/tmp")
		if err != nil {
			errCh <- err
			return
		}

		done <- wrapped
	}()

	require.Eventually(t, func() bool {
		for _, ev := range bus.events() {
			if ev.Topic == "sandbox.approve" {
				return true
			}
		}

		return false
	}, 2*time.Second, 50*time.Millisecond)

	// Simulate TUI publishing a denied event.
	bus.Publish(sdk.NewEvent("sandbox.denied", map[string]string{"command": "rm -rf /"}))

	select {
	case <-done:
		t.Fatal("expected error, got success")
	case err := <-errCh:
		assert.Contains(t, err.Error(), "denied")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for WrapCommand to complete")
	}
}

func TestWrapCommand_ModeAsk_PublishesCommandEvent(t *testing.T) {
	bus := newStubBus()
	s := &Sandbox{cfg: SandboxConfig{Mode: SandboxAsk}, headless: false}
	s.bus = bus

	require.NoError(t, s.Subscribe(bus))

	done := make(chan error, 1)

	go func() {
		_, err := s.WrapCommand("git status", "/tmp")
		done <- err
	}()

	// Wait specifically for sandbox.approve (sandbox.registered is also published
	// during Subscribe, so len > 0 is not sufficient).
	require.Eventually(t, func() bool {
		for _, ev := range bus.events() {
			if ev.Topic == "sandbox.approve" {
				return true
			}
		}

		return false
	}, 2*time.Second, 50*time.Millisecond, "expected sandbox.approve event")

	// Find the sandbox.approve event (not the sandbox.approved handler call).
	var approveEv *sdk.Event

	for _, ev := range bus.events() {
		if ev.Topic == "sandbox.approve" {
			approveEv = &ev
			break
		}
	}

	require.NotNil(t, approveEv)

	payload, ok := approveEv.Payload.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "git status", payload["command"])

	// Resolve to avoid goroutine leak.
	bus.Publish(sdk.NewEvent("sandbox.denied", map[string]string{"command": "git status"}))

	<-done
}

func TestSandbox_Subscribe_ApprovalHandlers(t *testing.T) {
	s := &Sandbox{cfg: SandboxConfig{Mode: SandboxAsk}, headless: false}
	bus := newStubBus()

	err := s.Subscribe(bus)
	require.NoError(t, err)

	_, hasApproved := bus.handlers["sandbox.approved"]
	assert.True(t, hasApproved, "expected sandbox.approved handler")

	_, hasDenied := bus.handlers["sandbox.denied"]
	assert.True(t, hasDenied, "expected sandbox.denied handler")
}

func TestNewSandbox_Headless_NilConfig(t *testing.T) {
	s, err := NewSandbox(nil, SandboxConfig{})
	require.NoError(t, err)
	assert.True(t, s.headless, "nil config should be headless")
}

func TestNewSandbox_Headless_HeadlessConfig(t *testing.T) {
	s, err := NewSandbox(sdk.HeadlessConfig{Config: sdk.FilePathConfig(""), Headless: true}, SandboxConfig{})
	require.NoError(t, err)
	assert.True(t, s.headless)
}

func TestNewSandbox_NotHeadless(t *testing.T) {
	s, err := NewSandbox(sdk.HeadlessConfig{Config: sdk.FilePathConfig(""), Headless: false}, SandboxConfig{})
	require.NoError(t, err)
	assert.False(t, s.headless)
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
		{"/project", "/project/", true},
		{"/project/file.go", "/project/", true},
		{"/other/file.go", "/project/", false},
	}

	for _, tt := range tests {
		got := pathMatches(tt.path, tt.pattern, "")
		assert.Equal(t, tt.want, got, "pathMatches(%q, %q)", tt.path, tt.pattern)
	}
}

func TestPathMatches_ProjectRoot(t *testing.T) {
	got := pathMatches("/project/file.go", ".", "/project")
	assert.True(t, got, "pathMatches with project root")

	got = pathMatches("/other/file.go", ".", "/project")
	assert.False(t, got, "pathMatches outside project root")
}

func TestPathMatches_EmptyPatternMatchesCWD(t *testing.T) {
	got := pathMatches("/project/file.go", "", "/project")
	assert.True(t, got, "empty pattern matches cwd")

	got = pathMatches("/other/file.go", "", "/project")
	assert.False(t, got, "empty pattern does not match outside cwd")
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
		got := isDeniedWrite(tt.path, "")
		assert.Equal(t, tt.want, got, "isDeniedWrite(%q)", tt.path)
	}
}

func TestIsDeniedWrite_ProjectRoot(t *testing.T) {
	tests := []struct {
		path string
		cwd  string
		want bool
	}{
		{"/project/.git/hooks/pre-commit", "/project", true},
		{"/project/.git/config", "/project", true},
		{"/project/.weave/settings.json", "/project", true},
		{"/project/src/main.go", "/project", false},
		{"/other/.git/hooks/pre-commit", "/project", false},
	}

	for _, tt := range tests {
		got := isDeniedWrite(tt.path, tt.cwd)
		assert.Equal(t, tt.want, got, "isDeniedWrite(%q, cwd=%q)", tt.path, tt.cwd)
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

func TestResolveAbs_SymlinkInPath(t *testing.T) {
	// Create a symlink inside a temp dir pointing to another temp dir.
	project := t.TempDir()
	target := t.TempDir()

	link := filepath.Join(project, "link-out")
	require.NoError(t, os.Symlink(target, link))

	// Request a non-existent path through the symlink.
	requested := filepath.Join(link, "newdir", "file.txt")
	resolved := resolveAbs(requested)

	// Resolve the target through any OS-level symlinks (macOS /var → /private/var).
	realTarget, err := filepath.EvalSymlinks(target)
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(realTarget, "newdir", "file.txt"), resolved,
		"should resolve symlink even when intermediate dirs don't exist")
}

func TestResolveAbs_SymlinkDotDotBypass(t *testing.T) {
	// Simulate the attack: /project/link -> /tmp/out/subdir
	// A path like link/../secret should resolve through the symlink,
	// not be cleaned to /project/secret before symlink evaluation.
	project := t.TempDir()
	outer := t.TempDir()
	subdir := filepath.Join(outer, "subdir")
	require.NoError(t, os.MkdirAll(subdir, 0o755))

	link := filepath.Join(project, "link")
	require.NoError(t, os.Symlink(subdir, link))

	// Use raw string concatenation (not filepath.Join) to preserve '..'.
	requested := link + "/../secret"
	resolved := resolveAbs(requested)

	// Expected: resolve link -> /tmp/out/subdir, then .. -> /tmp/out, then secret.
	realOuter, err := filepath.EvalSymlinks(outer)
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(realOuter, "secret"), resolved,
		"should follow symlink before processing .., not clean first")
}

func TestResolveAbs_ChainedSymlinkDotDotBypass(t *testing.T) {
	// link1 -> link2, link2 -> /tmp/out/subdir
	// /project/link1/../owned should resolve through both symlinks
	// to /tmp/out/owned, not /project/owned.
	project := t.TempDir()
	outer := t.TempDir()
	subdir := filepath.Join(outer, "subdir")
	require.NoError(t, os.MkdirAll(subdir, 0o755))

	link2 := filepath.Join(project, "link2")
	require.NoError(t, os.Symlink(subdir, link2))

	link1 := filepath.Join(project, "link1")
	require.NoError(t, os.Symlink("link2", link1)) // relative: link1 -> link2

	requested := link1 + "/../owned"
	resolved := resolveAbs(requested)

	realOuter, err := filepath.EvalSymlinks(outer)
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(realOuter, "owned"), resolved,
		"should resolve chained symlinks before processing ..")
}

func TestSandbox_ModeAsk_DenyWriteOverridesApproval(t *testing.T) {
	bus := newStubBus()
	s := &Sandbox{
		cfg: SandboxConfig{
			Mode:      SandboxAsk,
			DenyWrite: []string{"/project/secret"},
		},
		headless: false,
	}
	s.bus = bus
	require.NoError(t, s.Subscribe(bus))

	// DenyWrite should block even in ask mode, before prompting.
	assert.False(t, s.AllowWrite("/project/secret/key"),
		"deny_write should block in ask mode without prompting")
	// Verify no approve event was published (prompt was never shown).
	for _, ev := range bus.events() {
		assert.NotEqual(t, "sandbox.approve", ev.Topic,
			"should not prompt for deny_write blocked path")
	}
}
