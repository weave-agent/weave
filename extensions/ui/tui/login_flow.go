package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/weave-agent/weave-tui/components/messages"
	"github.com/weave-agent/weave/sdk"
	sdkmodel "github.com/weave-agent/weave/sdk/model"

	tea "charm.land/bubbletea/v2"
)

// LoginFlowResultMsg is sent when an asynchronous OAuth login flow completes.
type LoginFlowResultMsg struct {
	Provider   string
	Credential sdk.OAuthCredential
	Error      error
	Gen        int // generation counter to detect stale results from canceled flows
}

// LoginAuthURLMsg is sent when the authorization URL has been generated for an
// OAuth authorization code flow. The UI should update the login dialog with the
// full URL and then issue completeOAuthFlowCmd to finish the flow.
type LoginAuthURLMsg struct {
	Provider string
	URL      string
	Handle   *sdk.AuthorizationFlowHandle
	Gen      int
}

// runOAuthFlowCmd returns a tea.Cmd that starts the OAuth authorization code
// flow, opens the browser, and returns a LoginAuthURLMsg containing the full
// authorization URL. The caller must then issue completeOAuthFlowCmd with the
// handle to finish the flow.
func runOAuthFlowCmd(parentCtx context.Context, provider sdk.OAuthProvider, gen int) tea.Cmd {
	return func() tea.Msg {
		authCodeURL, handle, err := sdk.StartAuthorizationCodeFlow(parentCtx, provider.AuthURL, provider.TokenURL, provider.ClientID, provider.RedirectURI, provider.Scopes, provider.ExtraAuthParams)
		if err != nil {
			return LoginFlowResultMsg{
				Provider: provider.ID,
				Error:    err,
				Gen:      gen,
			}
		}

		return LoginAuthURLMsg{
			Provider: provider.ID,
			URL:      authCodeURL,
			Handle:   handle,
			Gen:      gen,
		}
	}
}

// completeOAuthFlowCmd returns a tea.Cmd that waits for the OAuth callback and
// exchanges the authorization code for tokens. It must be called after
// runOAuthFlowCmd returns a LoginAuthURLMsg.
func completeOAuthFlowCmd(parentCtx context.Context, handle *sdk.AuthorizationFlowHandle, providerID string, gen int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parentCtx, 2*time.Minute)
		defer cancel()

		cred, err := sdk.CompleteAuthorizationCodeFlow(ctx, handle)

		return LoginFlowResultMsg{
			Provider:   providerID,
			Credential: cred,
			Error:      err,
			Gen:        gen,
		}
	}
}

// pollDeviceCodeCmd returns a tea.Cmd that polls the token endpoint for a
// device code flow and returns a LoginFlowResultMsg.
func pollDeviceCodeCmd(parentCtx context.Context, providerID, deviceCode string, intervalSecs int, tokenURL, clientID string, gen int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parentCtx, 2*time.Minute)
		defer cancel()

		tokenResp, err := sdk.PollDeviceToken(ctx, tokenURL, clientID, deviceCode, intervalSecs)
		if err != nil {
			return LoginFlowResultMsg{
				Provider: providerID,
				Error:    err,
				Gen:      gen,
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
			Gen:        gen,
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

	if msg.Credential.AccessToken == "" {
		am := messages.NewAssistantMessage()
		am.Finalize(fmt.Sprintf("OAuth login failed for %s: received empty access token", displayNameForProvider(msg.Provider)))
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
			m.footer = m.footer.SetReasoning(modelReasoning(cur))
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
