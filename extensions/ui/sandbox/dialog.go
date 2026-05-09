package sandboxui

import (
	"strings"

	"weave/sdk"
)

const (
	approveOption      = "Approve"
	denyOption         = "Deny"
	trustSessionOption = "Trust for session"
	keyCommand         = "command"
)

// ApproveDialog handles ask-mode approval flow by listening for sandbox.approve
// bus events, showing a selector dialog via sdk.UI, and publishing the response
// back on the bus.
type ApproveDialog struct {
	ui  sdk.UI
	bus sdk.Bus
}

// RegisterWithBus wires the approve dialog to the event bus.
// Implements sdk.UIExtensionWithBus.
func (d *ApproveDialog) RegisterWithBus(ui sdk.UI, bus sdk.Bus) {
	d.ui = ui
	d.bus = bus

	bus.On("sandbox.approve", func(ev sdk.Event) error {
		d.handleApproval(ev)
		return nil
	})
}

// handleApproval shows a selector dialog for the command and publishes the response.
func (d *ApproveDialog) handleApproval(ev sdk.Event) {
	if d.ui == nil || d.bus == nil {
		return
	}

	payload, ok := ev.Payload.(map[string]string)
	if !ok {
		return
	}

	command := payload[keyCommand]
	if command == "" {
		return
	}

	items := []string{
		approveOption,
		denyOption,
		trustSessionOption,
	}

	title := formatApprovalTitle(command)

	idx, err := d.ui.Select(title, items)
	if err != nil {
		d.bus.Publish(sdk.NewEvent("sandbox.denied", map[string]string{
			keyCommand: command,
		}))

		return
	}

	switch idx {
	case 0:
		d.bus.Publish(sdk.NewEvent("sandbox.approved", map[string]string{
			keyCommand: command,
			"trust":    "false",
		}))
	case 2:
		d.bus.Publish(sdk.NewEvent("sandbox.approved", map[string]string{
			keyCommand: command,
			"trust":    "true",
		}))
		d.bus.Publish(sdk.NewEvent("sandbox.trust", map[string]string{
			"pattern": extractPattern(command),
		}))
	default:
		d.bus.Publish(sdk.NewEvent("sandbox.denied", map[string]string{
			keyCommand: command,
		}))
	}
}

// formatApprovalTitle creates the dialog title from the command text.
func formatApprovalTitle(command string) string {
	const maxLen = 60

	if len(command) > maxLen {
		return "Sandbox: allow command?\n\n" + command[:maxLen] + "..."
	}

	return "Sandbox: allow command?\n\n" + command
}

// extractPattern extracts a trustable pattern from a command.
// For now, uses the command's base executable as the pattern.
func extractPattern(command string) string {
	cmd := strings.TrimSpace(command)

	if idx := strings.IndexAny(cmd, " \t"); idx > 0 {
		return cmd[:idx] + " *"
	}

	return cmd
}
