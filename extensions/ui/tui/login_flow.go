package tui

import (
	"context"
	"fmt"
	"time"

	"weave/ext/ui/tui/components/messages"
	"weave/sdk"
	sdkmodel "weave/sdk/model"

	tea "charm.land/bubbletea/v2"
)

// LoginFlowResultMsg is sent when an asynchronous OAuth login flow completes.
type LoginFlowResultMsg struct {
	Provider   string
	Credential sdk.OAuthCredential
	Error      error
}

// runOAuthFlowCmd returns a tea.Cmd that executes the authorization code flow
// for the given OAuth provider and returns a LoginFlowResultMsg.
func runOAuthFlowCmd(provider sdk.OAuthProvider) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		cred, err := sdk.RunAuthorizationCodeFlow(ctx, provider.AuthURL, provider.TokenURL, provider.ClientID, provider.Scopes)

		return LoginFlowResultMsg{
			Provider:   provider.ID,
			Credential: cred,
			Error:      err,
		}
	}
}

func (m Model) onLoginFlowResult(msg LoginFlowResultMsg) (tea.Model, tea.Cmd) {
	if msg.Error != nil {
		am := messages.NewAssistantMessage()
		am.Finalize(fmt.Sprintf("OAuth login failed for %s: %v", displayNameForProvider(msg.Provider), msg.Error))
		m.chat = m.chat.AddItem(am)

		return m, nil
	}

	if err := sdk.SetOAuthCredential(msg.Provider, msg.Credential); err != nil {
		am := messages.NewAssistantMessage()
		am.Finalize(fmt.Sprintf("Failed to save OAuth credentials for %s: %v", displayNameForProvider(msg.Provider), err))
		m.chat = m.chat.AddItem(am)

		return m, nil
	}

	// Update in-memory auth status so the provider is immediately usable.
	sdkmodel.SetProviderAuth(msg.Provider, true)

	am := messages.NewAssistantMessage()
	am.Finalize(fmt.Sprintf("Successfully logged in to %s.", displayNameForProvider(msg.Provider)))
	m.chat = m.chat.AddItem(am)

	// If we were in noConfigured state, re-evaluate now that auth exists.
	if m.noConfigured {
		models := listModels()
		if len(models) > 0 {
			m.noConfigured = false
			cur := currentModel(models, m.ps)
			m.currentModel = cur
			m.footer = m.footer.SetModel(cur.Model, cur.Provider)
			m.footer = m.footer.SetReasoning(modelReasoning(cur.Model))
		}
	}

	var cmds []tea.Cmd

	if m.bus != nil {
		cmds = append(cmds, PublishAuthLoginSuccess(m.bus, msg.Provider))
	}

	return m, tea.Batch(cmds...)
}
