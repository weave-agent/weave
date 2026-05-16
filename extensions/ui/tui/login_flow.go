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

// runOAuthFlowCmd returns a tea.Cmd that executes the OAuth flow for the given
// provider and returns a LoginFlowResultMsg. For authorization code flow, it
// starts a callback server and opens the browser. For device code flow, the
// caller must have already requested the device code and should use
// pollDeviceCodeCmd for polling.
func runOAuthFlowCmd(parentCtx context.Context, provider sdk.OAuthProvider) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parentCtx, 2*time.Minute)
		defer cancel()

		cred, err := sdk.RunAuthorizationCodeFlow(ctx, provider.AuthURL, provider.TokenURL, provider.ClientID, provider.RedirectURI, provider.Scopes, provider.ExtraAuthParams)

		return LoginFlowResultMsg{
			Provider:   provider.ID,
			Credential: cred,
			Error:      err,
		}
	}
}

// pollDeviceCodeCmd returns a tea.Cmd that polls the token endpoint for a
// device code flow and returns a LoginFlowResultMsg.
func pollDeviceCodeCmd(parentCtx context.Context, providerID, deviceCode string, intervalSecs int, tokenURL, clientID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parentCtx, 2*time.Minute)
		defer cancel()

		tokenResp, err := sdk.PollDeviceToken(ctx, tokenURL, clientID, deviceCode, intervalSecs)
		if err != nil {
			return LoginFlowResultMsg{
				Provider: providerID,
				Error:    err,
			}
		}

		cred := sdk.OAuthCredential{
			AccessToken:  tokenResp.AccessToken,
			RefreshToken: tokenResp.RefreshToken,
			TokenType:    tokenResp.TokenType,
		}

		if tokenResp.ExpiresIn > 0 {
			cred.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
		}

		return LoginFlowResultMsg{
			Provider:   providerID,
			Credential: cred,
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

		// If we transitioned out of noConfigured, publish model.change so the
		// agent loop switches to the newly available provider.
		if !m.noConfigured {
			cmds = append(cmds, PublishModelChange(m.bus, m.currentModel))
		}
	}

	return m, tea.Batch(cmds...)
}
