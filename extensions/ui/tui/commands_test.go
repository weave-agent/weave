package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/weave-agent/weave-tui/components/messages"
	"github.com/weave-agent/weave/bus"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandRegistry_BuiltinCommands(t *testing.T) {
	b := bus.New()
	defer b.Close()

	r := NewCommandRegistry(b, "")

	names := r.Names()
	assert.Contains(t, names, "/new")
	assert.Contains(t, names, "/clear")
	assert.Contains(t, names, "/quit")
	assert.Contains(t, names, "/help")
	assert.Contains(t, names, "/compact")
	assert.Contains(t, names, "/name")
	assert.Contains(t, names, "/login")
	assert.Contains(t, names, "/logout")
}

func TestCommandRegistry_DispatchNonCommand(t *testing.T) {
	b := bus.New()
	defer b.Close()

	r := NewCommandRegistry(b, "")

	handled, result := r.Dispatch("hello world")
	assert.False(t, handled)
	assert.Equal(t, CommandResult{}, result)
}

func TestCommandRegistry_DispatchQuit(t *testing.T) {
	b := bus.New()
	defer b.Close()

	r := NewCommandRegistry(b, "")

	handled, result := r.Dispatch("/quit")
	require.True(t, handled)
	assert.True(t, result.Quit)
	assert.False(t, result.ClearChat)
}

func TestCommandRegistry_DispatchNew(t *testing.T) {
	b := bus.New()
	defer b.Close()

	r := NewCommandRegistry(b, "")

	handled, result := r.Dispatch("/new")
	require.True(t, handled)
	assert.True(t, result.ClearChat)
	assert.True(t, result.ResetPrompt)
	assert.False(t, result.Quit)
}

func TestCommandRegistry_DispatchClearAliasForNew(t *testing.T) {
	b := bus.New()
	defer b.Close()

	r := NewCommandRegistry(b, "")

	handled, result := r.Dispatch("/clear")
	require.True(t, handled)
	assert.True(t, result.ClearChat)
	assert.True(t, result.ResetPrompt)

	// Same result as /new
	handledNew, resultNew := r.Dispatch("/new")
	assert.Equal(t, resultNew, result)
	assert.Equal(t, handledNew, handled)
}

func TestCommandRegistry_DispatchHelp(t *testing.T) {
	b := bus.New()
	defer b.Close()

	r := NewCommandRegistry(b, "")

	handled, result := r.Dispatch("/help")
	require.True(t, handled)
	assert.Contains(t, result.Notify, "Available commands")
	assert.Contains(t, result.Notify, "/quit")
	assert.Contains(t, result.Notify, "/new")
}

func TestCommandRegistry_DispatchCompact(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := subscribeToChan(b, topicSteer)

	r := NewCommandRegistry(b, "")

	handled, result := r.Dispatch("/compact")
	require.True(t, handled)
	assert.NotNil(t, result.Command)

	// Execute the command
	msg := result.Command()
	assert.Nil(t, msg)

	select {
	case evt := <-ch:
		assert.Equal(t, "compact", evt.Payload)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for compact event")
	}
}

func TestCommandRegistry_DispatchCompactWithArgs(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := subscribeToChan(b, topicSteer)

	r := NewCommandRegistry(b, "")

	handled, result := r.Dispatch("/compact focus on the auth refactor")
	require.True(t, handled)
	assert.NotNil(t, result.Command)

	msg := result.Command()
	assert.Nil(t, msg)

	select {
	case evt := <-ch:
		assert.Equal(t, "compact focus on the auth refactor", evt.Payload)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for compact event with args")
	}
}

func TestCommandRegistry_DispatchNameWithArgs(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := subscribeToChan(b, topicSteer)

	r := NewCommandRegistry(b, "")

	handled, result := r.Dispatch("/name my session")
	require.True(t, handled)
	assert.NotNil(t, result.Command)

	msg := result.Command()
	assert.Nil(t, msg)

	evt := <-ch
	assert.Equal(t, "name my session", evt.Payload)
}

func TestCommandRegistry_DispatchUnknownCommand(t *testing.T) {
	b := bus.New()
	defer b.Close()

	r := NewCommandRegistry(b, "")

	handled, result := r.Dispatch("/unknown")
	require.True(t, handled)
	assert.Contains(t, result.Notify, "unknown command: /unknown")
}

func TestCommandRegistry_DispatchCommandWithArgs(t *testing.T) {
	b := bus.New()
	defer b.Close()

	r := NewCommandRegistry(b, "")

	// /name with args should work
	handled, result := r.Dispatch("/name test")
	require.True(t, handled)
	assert.NotNil(t, result.Command)

	// /quit with extra args still quits
	handled, result = r.Dispatch("/quit now")
	require.True(t, handled)
	assert.True(t, result.Quit)
}

func TestCommandRegistry_NamesSorted(t *testing.T) {
	b := bus.New()
	defer b.Close()

	r := NewCommandRegistry(b, "")

	names := r.Names()
	for i := 1; i < len(names); i++ {
		assert.LessOrEqual(t, names[i-1], names[i], "names should be sorted: %v", names)
	}
}

func TestCommandRegistry_Lookup(t *testing.T) {
	b := bus.New()
	defer b.Close()

	r := NewCommandRegistry(b, "")

	info, ok := r.Lookup("/quit")
	require.True(t, ok)
	assert.Equal(t, "/quit", info.Name)
	assert.Equal(t, "Exit weave", info.Description)

	_, ok = r.Lookup("/nonexistent")
	assert.False(t, ok)
}

func TestCommandRegistry_Register(t *testing.T) {
	b := bus.New()
	defer b.Close()

	r := NewCommandRegistry(b, "")

	called := false

	r.Register("/custom", "custom command", false, func(args string) CommandResult {
		called = true

		assert.Equal(t, "arg1", args)

		return CommandResult{}
	})

	names := r.Names()
	assert.Contains(t, names, "/custom")

	handled, _ := r.Dispatch("/custom arg1")
	require.True(t, handled)
	assert.True(t, called)
}

func TestCommandRegistry_AcceptsFilesDefaultsToFalse(t *testing.T) {
	b := bus.New()
	defer b.Close()

	r := NewCommandRegistry(b, "")

	// All built-in commands should have AcceptsFiles = false
	for _, name := range r.Names() {
		info, ok := r.Lookup(name)
		require.True(t, ok, "should find %s", name)
		assert.False(t, info.AcceptsFiles, "%s should not accept files by default", name)
	}

	// Register with explicit true
	r.Register("/filecmd", "accepts files", true, func(_ string) CommandResult { return CommandResult{} })
	info, ok := r.Lookup("/filecmd")
	require.True(t, ok)
	assert.True(t, info.AcceptsFiles, "/filecmd should accept files")

	// Register with explicit false
	r.Register("/nocmd", "no files", false, func(_ string) CommandResult { return CommandResult{} })
	info, ok = r.Lookup("/nocmd")
	require.True(t, ok)
	assert.False(t, info.AcceptsFiles, "/nocmd should not accept files")
}

func TestParseCommand(t *testing.T) {
	tests := []struct {
		input string
		name  string
		args  string
	}{
		{"/quit", "/quit", ""},
		{"/quit ", "/quit", ""},
		{"/name my session", "/name", "my session"},
		{"/help  ", "/help", ""},
		{"  /compact  ", "/compact", ""},
		{"/name  extra  spaces  ", "/name", "extra  spaces"},
	}

	for _, tt := range tests {
		name, args := parseCommand(tt.input)
		assert.Equal(t, tt.name, name, "name for %q", tt.input)
		assert.Equal(t, tt.args, args, "args for %q", tt.input)
	}
}

func TestCommandRegistry_HelpListsAll(t *testing.T) {
	b := bus.New()
	defer b.Close()

	r := NewCommandRegistry(b, "")

	r.Register("/testcmd", "a test", false, func(_ string) CommandResult { return CommandResult{} })

	help := r.helpText()
	assert.Contains(t, help, "/testcmd")
	assert.Contains(t, help, "a test")
}

func TestModel_SlashCommandQuit(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, 10)

	model, cmd := m.onSubmit("/quit")
	require.NotNil(t, cmd)
	assert.Empty(t, m.chat.Items())

	// Verify quit command
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok)

	_ = model
}

func TestModel_SlashCommandNewClearsChat(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, 10)

	// Add some messages first
	m.AddUserMessage("hello")
	m.prompted = true

	model, _ := m.onSubmit("/new")
	m2 := model.(Model)

	assert.Empty(t, m2.chat.Items())
	assert.False(t, m2.prompted)
	assert.Empty(t, m2.toolPanels)
}

func TestModel_SlashCommandClear(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, 10)

	m.AddUserMessage("hello")
	m.prompted = true

	model, _ := m.onSubmit("/clear")
	m2 := model.(Model)

	assert.Empty(t, m2.chat.Items())
	assert.False(t, m2.prompted)
}

func TestModel_SlashCommandHelpShowsMessage(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, 10)

	model, _ := m.onSubmit("/help")
	m2 := model.(Model)

	items := m2.chat.Items()
	require.Len(t, items, 1)

	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "Available commands")

	// Verify the message renders with the role indicator
	view := am.View(80)
	assert.Contains(t, view, "◆")
}

func TestModel_RegularSubmitPublishesPrompt(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := subscribeToChan(b, topicPrompt)

	m := newModel(b, nil, nil, nil)
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, 10)

	model, cmd := m.onSubmit("hello world")
	require.NotNil(t, cmd)

	// onSubmit returns a spinner tick cmd to start the render loop
	msg := cmd()
	assert.NotNil(t, msg)

	evt := <-ch
	assert.Equal(t, "hello world", evt.Payload)

	m2 := model.(Model)
	assert.True(t, m2.prompted)
}

func TestModel_RegularSubmitFollowup(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := subscribeToChan(b, topicFollowup)

	m := newModel(b, nil, nil, nil)
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, 10)
	m.prompted = true

	model, cmd := m.onSubmit("follow up text")
	require.NotNil(t, cmd)

	// onSubmit returns a spinner tick cmd to start the render loop
	msg := cmd()
	assert.NotNil(t, msg)

	evt := <-ch
	assert.Equal(t, "follow up text", evt.Payload)

	m2 := model.(Model)
	assert.True(t, m2.prompted)
}

func TestModel_UnknownCommandShowsError(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, 10)

	model, _ := m.onSubmit("/bogus")
	m2 := model.(Model)

	items := m2.chat.Items()
	require.Len(t, items, 1)

	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "unknown command: /bogus")

	// Verify the message renders with the role indicator
	view := am.View(80)
	assert.Contains(t, view, "◆")
}

func TestModel_ThinkingCommandRegistered(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	_, ok := m.commands.Lookup("/thinking")
	assert.True(t, ok, "/thinking command should be registered")
}

func TestModel_ThinkingCommandInHelp(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, 10)

	model, _ := m.onSubmit("/help")
	m2 := model.(Model)

	items := m2.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "/thinking")
}

func TestCommandRegistry_ReloadRegistered(t *testing.T) {
	b := bus.New()
	defer b.Close()

	r := NewCommandRegistry(b, "")

	names := r.Names()
	assert.Contains(t, names, "/reload")

	info, ok := r.Lookup("/reload")
	require.True(t, ok)
	assert.Equal(t, "Rebuild extensions and restart", info.Description)
}

func TestReloadCmd_NoLauncherPath(t *testing.T) {
	t.Setenv("WEAVE_LAUNCHER_PATH", "")
	t.Setenv("WEAVE_BUILD_HASH", "")
	t.Setenv("WEAVE_ORIG_ARGS", "")

	cmd := reloadCmd()
	msg := cmd()

	notify, ok := msg.(notifyMsg)
	require.True(t, ok, "expected notifyMsg when WEAVE_LAUNCHER_PATH is empty")
	assert.Contains(t, notify.message, "not available")
}

func TestReloadCmd_RemovesCacheDir(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	testHash := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	cacheDir := filepath.Join(fakeHome, ".weave", "bin", testHash)

	// Create a fake cache dir to verify it gets removed.
	require.NoError(t, os.MkdirAll(cacheDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "weave"), []byte("fake"), 0o600))

	origArgs, _ := json.Marshal([]string{"weave", "arg1"})

	t.Setenv("WEAVE_LAUNCHER_PATH", "/usr/local/bin/weave")
	t.Setenv("WEAVE_BUILD_HASH", testHash)
	t.Setenv("WEAVE_ORIG_ARGS", string(origArgs))

	cmd := reloadCmd()
	msg := cmd()

	reload, ok := msg.(reloadMsg)
	require.True(t, ok, "expected reloadMsg")
	assert.Equal(t, "/usr/local/bin/weave", reload.launcherPath)
	assert.Equal(t, []string{"weave", "arg1"}, reload.origArgs)

	// Verify cache dir was removed.
	_, statErr := os.Stat(cacheDir)
	assert.True(t, os.IsNotExist(statErr), "cache dir should be removed")
}

func TestReloadCmd_MissingOrigArgs(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	testHash := "0000000000000000000000000000000000000000000000000000000000000001"
	cacheDir := filepath.Join(fakeHome, ".weave", "bin", testHash)
	_ = os.MkdirAll(cacheDir, 0o750)

	t.Setenv("WEAVE_LAUNCHER_PATH", "/usr/bin/weave")
	t.Setenv("WEAVE_BUILD_HASH", testHash)
	t.Setenv("WEAVE_ORIG_ARGS", "")

	cmd := reloadCmd()
	msg := cmd()

	reload, ok := msg.(reloadMsg)
	require.True(t, ok)
	assert.Equal(t, []string{"/usr/bin/weave"}, reload.origArgs)
}

func TestReloadCmd_EmptyBuildHash(t *testing.T) {
	t.Setenv("WEAVE_LAUNCHER_PATH", "/usr/bin/weave")
	t.Setenv("WEAVE_BUILD_HASH", "")
	t.Setenv("WEAVE_ORIG_ARGS", `["weave"]`)

	cmd := reloadCmd()
	msg := cmd()

	reload, ok := msg.(reloadMsg)
	require.True(t, ok)
	assert.Equal(t, "/usr/bin/weave", reload.launcherPath)
}

// TestReloadCmd_RejectsPathTraversal verifies a malicious WEAVE_BUILD_HASH
// value is rejected before being joined into a filesystem path. Without the
// validation, "../../victim" would escape ~/.weave/bin and let os.RemoveAll
// delete unrelated files.
func TestReloadCmd_RejectsPathTraversal(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	// Create a sibling directory that must NOT be removed.
	victim := filepath.Join(fakeHome, "victim")
	require.NoError(t, os.MkdirAll(victim, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(victim, "data"), []byte("important"), 0o600))

	t.Setenv("WEAVE_LAUNCHER_PATH", "/usr/bin/weave")
	t.Setenv("WEAVE_BUILD_HASH", "../../victim")
	t.Setenv("WEAVE_ORIG_ARGS", `["weave"]`)

	cmd := reloadCmd()
	msg := cmd()

	notify, ok := msg.(notifyMsg)
	require.True(t, ok, "expected notifyMsg for invalid hash, got %T", msg)
	assert.Contains(t, notify.message, "invalid build hash")

	// Victim directory must still exist.
	_, err := os.Stat(victim)
	assert.NoError(t, err, "victim directory must not be deleted")
}

func TestIsSHA256Hex(t *testing.T) {
	valid := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	assert.True(t, isSHA256Hex(valid))

	tests := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"too short", "abc123"},
		{"too long", valid + "0"},
		{"path traversal", "../../victim"},
		{"uppercase letters", strings.ToUpper(valid)},
		{"non-hex char", strings.Repeat("g", 64)},
		{"slash", strings.Repeat("a", 63) + "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, isSHA256Hex(tt.in))
		})
	}
}

func TestCommandRegistry_DispatchLogin(t *testing.T) {
	b := bus.New()
	defer b.Close()

	r := NewCommandRegistry(b, "")

	handled, result := r.Dispatch("/login")
	require.True(t, handled)
	assert.NotNil(t, result.Command)

	msg := result.Command()
	loginResult, ok := msg.(LoginListResultMsg)
	require.True(t, ok, "expected LoginListResultMsg, got %T", msg)
	assert.NotNil(t, loginResult.Providers)
}

func TestCommandRegistry_DispatchLogout(t *testing.T) {
	b := bus.New()
	defer b.Close()

	r := NewCommandRegistry(b, "")

	handled, result := r.Dispatch("/logout")
	require.True(t, handled)
	assert.NotNil(t, result.Command)

	msg := result.Command()
	logoutResult, ok := msg.(LogoutListResultMsg)
	require.True(t, ok, "expected LogoutListResultMsg, got %T", msg)
	assert.NotNil(t, logoutResult.Providers)
}

func TestCommandRegistry_LoginLogoutInHelp(t *testing.T) {
	b := bus.New()
	defer b.Close()

	r := NewCommandRegistry(b, "")

	help := r.helpText()
	assert.Contains(t, help, "/login")
	assert.Contains(t, help, "/logout")
}
