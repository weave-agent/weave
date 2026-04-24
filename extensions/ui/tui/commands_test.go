package tui

import (
	"testing"

	"weave/bus"
	"weave/ext/ui/tui/components/messages"

	tea "github.com/charmbracelet/bubbletea"
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

	ch := b.Subscribe(topicSteer)

	r := NewCommandRegistry(b, "")

	handled, result := r.Dispatch("/compact")
	require.True(t, handled)
	assert.NotNil(t, result.Command)

	// Execute the command
	msg := result.Command()
	assert.Nil(t, msg)

	evt := <-ch
	assert.Equal(t, "compact", evt.Payload)
}

func TestCommandRegistry_DispatchNameWithArgs(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := b.Subscribe(topicSteer)

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

	r.Register("/custom", "custom command", func(args string) CommandResult {
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

	r.Register("/testcmd", "a test", func(_ string) CommandResult { return CommandResult{} })

	help := r.helpText()
	assert.Contains(t, help, "/testcmd")
	assert.Contains(t, help, "a test")
}

func TestModel_SlashCommandQuit(t *testing.T) {
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, 10)

	model, _ := m.onSubmit("/help")
	m2 := model.(Model)

	items := m2.chat.Items()
	require.Len(t, items, 1)

	_, ok := items[0].(*messages.AssistantMessage)
	assert.True(t, ok)

	view := m2.View()
	assert.Contains(t, view, "Available commands")
}

func TestModel_RegularSubmitPublishesPrompt(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := b.Subscribe(topicPrompt)

	m := newModel(b, nil, nil)
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, 10)

	model, cmd := m.onSubmit("hello world")
	require.NotNil(t, cmd)

	msg := cmd()
	assert.Nil(t, msg)

	evt := <-ch
	assert.Equal(t, "hello world", evt.Payload)

	m2 := model.(Model)
	assert.True(t, m2.prompted)
}

func TestModel_RegularSubmitFollowup(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := b.Subscribe(topicFollowup)

	m := newModel(b, nil, nil)
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, 10)
	m.prompted = true

	model, cmd := m.onSubmit("follow up text")
	require.NotNil(t, cmd)

	msg := cmd()
	assert.Nil(t, msg)

	evt := <-ch
	assert.Equal(t, "follow up text", evt.Payload)

	m2 := model.(Model)
	assert.True(t, m2.prompted)
}

func TestModel_UnknownCommandShowsError(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, 10)

	model, _ := m.onSubmit("/bogus")
	m2 := model.(Model)

	view := m2.View()
	assert.Contains(t, view, "unknown command: /bogus")
}

func TestModel_ThinkingCommandRegistered(t *testing.T) {
	m := newModel(nil, nil, nil)
	_, ok := m.commands.Lookup("/thinking")
	assert.True(t, ok, "/thinking command should be registered")
}

func TestModel_ThinkingCommandInHelp(t *testing.T) {
	m := newModel(nil, nil, nil)
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
