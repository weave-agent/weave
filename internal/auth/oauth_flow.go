package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const pkceMethodS256 = "S256"

// PKCE holds a generated code verifier and its challenge.
type PKCE struct {
	Verifier  string
	Challenge string
	Method    string
}

// GeneratePKCE creates a new PKCE verifier and S256 challenge.
func GeneratePKCE() (PKCE, error) {
	// Code verifier: 43-128 chars of [A-Za-z0-9-._~]
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		return PKCE{}, fmt.Errorf("generate random bytes: %w", err)
	}

	verifier := base64.RawURLEncoding.EncodeToString(b)

	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])

	return PKCE{
		Verifier:  verifier,
		Challenge: challenge,
		Method:    pkceMethodS256,
	}, nil
}

// AuthorizationCodeURL builds the OAuth authorization URL with PKCE parameters.
func AuthorizationCodeURL(authURL, clientID, redirectURI, state string, pkce PKCE, scopes []string, extraParams map[string]string) string {
	u, err := url.Parse(authURL)
	if err != nil {
		// Fallback: return the raw URL with query params appended manually
		return authURL
	}

	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	q.Set("code_challenge", pkce.Challenge)
	q.Set("code_challenge_method", pkce.Method)

	if len(scopes) > 0 {
		q.Set("scope", strings.Join(scopes, " "))
	}

	for k, v := range extraParams {
		q.Set(k, v)
	}

	u.RawQuery = q.Encode()

	return u.String()
}

// GenerateState creates a random state parameter for CSRF protection.
func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

// OpenBrowser opens the given URL in the system default browser.
func OpenBrowser(browserURL string) error {
	var (
		cmd  string
		args = make([]string, 0, 1)
	)

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	default:
		cmd = "xdg-open"
	}

	args = append(args, browserURL)
	c := exec.Command(cmd, args...) //nolint:noctx // Browser command doesn't benefit from context cancellation

	if err := c.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}

	return nil
}

// TokenResponse is the parsed JSON response from the token endpoint.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// ExchangeAuthorizationCode exchanges an authorization code for tokens.
func ExchangeAuthorizationCode(ctx context.Context, tokenURL, clientID, code, redirectURI string, pkce PKCE) (TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", clientID)
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("code_verifier", pkce.Verifier)

	return postTokenForm(ctx, tokenURL, data, "token exchange")
}

// RefreshToken exchanges a refresh token for a new access token.
func RefreshToken(ctx context.Context, tokenURL, clientID, refreshToken string) (TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", clientID)
	data.Set("refresh_token", refreshToken)

	return postTokenForm(ctx, tokenURL, data, "token refresh")
}

func postTokenForm(ctx context.Context, tokenURL string, data url.Values, operation string) (TokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return TokenResponse{}, fmt.Errorf("create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("token request failed: %w", err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("read token response: %w", err)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return TokenResponse{}, fmt.Errorf("parse token response: %w", err)
	}

	if resp.StatusCode >= 400 || tokenResp.Error != "" {
		msg := tokenResp.Error
		if tokenResp.ErrorDesc != "" {
			msg += ": " + tokenResp.ErrorDesc
		}

		if msg == "" {
			msg = fmt.Sprintf("token endpoint returned %d", resp.StatusCode)
		}

		return TokenResponse{}, fmt.Errorf("%s failed: %s", operation, msg)
	}

	return tokenResp, nil
}

// OAuthFlowResult holds the outcome of an authorization code flow.
type OAuthFlowResult struct {
	Credential OAuthCredential
	Error      error
}

// DeviceCodeResponse holds the result from the device authorization endpoint
// (RFC 8628).
type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURL         string `json:"verification_url"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
	Error                   string `json:"error"`
	ErrorDesc               string `json:"error_description"`
}

// VerificationURLOrURI returns the verification URL or URI, whichever is
// present. It prefers verification_uri_complete, then verification_uri, then
// verification_url.
func (r DeviceCodeResponse) VerificationURLOrURI() string {
	if r.VerificationURIComplete != "" {
		return r.VerificationURIComplete
	}

	if r.VerificationURI != "" {
		return r.VerificationURI
	}

	return r.VerificationURL
}

// RequestDeviceCode requests a device code from the device authorization
// endpoint (RFC 8628 section 3.1).
func RequestDeviceCode(ctx context.Context, deviceCodeURL, clientID string, scopes []string) (DeviceCodeResponse, error) {
	data := url.Values{}
	data.Set("client_id", clientID)

	if len(scopes) > 0 {
		data.Set("scope", strings.Join(scopes, " "))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceCodeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return DeviceCodeResponse{}, fmt.Errorf("create device code request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return DeviceCodeResponse{}, fmt.Errorf("device code request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return DeviceCodeResponse{}, fmt.Errorf("read device code response: %w", err)
	}

	var devResp DeviceCodeResponse
	if err := json.Unmarshal(body, &devResp); err != nil {
		return DeviceCodeResponse{}, fmt.Errorf("parse device code response: %w", err)
	}

	if resp.StatusCode >= 400 || devResp.Error != "" {
		msg := devResp.Error
		if devResp.ErrorDesc != "" {
			msg += ": " + devResp.ErrorDesc
		}

		if msg == "" {
			msg = fmt.Sprintf("device code endpoint returned %d", resp.StatusCode)
		}

		return DeviceCodeResponse{}, fmt.Errorf("device code request failed: %s", msg)
	}

	return devResp, nil
}

// PollDeviceToken polls the token endpoint for a device code flow (RFC 8628
// section 3.4) until the user authorizes the device, an error occurs, or the
// context is canceled.
func PollDeviceToken(ctx context.Context, tokenURL, clientID, deviceCode string, intervalSecs int) (TokenResponse, error) {
	if intervalSecs <= 0 {
		intervalSecs = 5
	}

	ticker := time.NewTicker(time.Duration(intervalSecs) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return TokenResponse{}, errors.New("device code polling timed out or was canceled")

		case <-ticker.C:
			tokenResp, cont, err := pollDeviceTokenOnce(ctx, tokenURL, clientID, deviceCode)
			if err != nil {
				return TokenResponse{}, err
			}

			if !cont {
				if tokenResp.AccessToken == "" {
					return TokenResponse{}, errors.New("device code authorization response missing access_token")
				}

				return tokenResp, nil
			}

			if tokenResp.Error == "slow_down" {
				intervalSecs += 5
				ticker.Reset(time.Duration(intervalSecs) * time.Second)
			}
		}
	}
}

// pollDeviceTokenOnce makes a single token poll request for device code flow.
// Returns (tokenResponse, shouldContinue, error). When shouldContinue is true,
// the caller should poll again (authorization_pending or slow_down).
func pollDeviceTokenOnce(ctx context.Context, tokenURL, clientID, deviceCode string) (TokenResponse, bool, error) {
	data := url.Values{}
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	data.Set("client_id", clientID)
	data.Set("device_code", deviceCode)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return TokenResponse{}, false, fmt.Errorf("create token poll request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return TokenResponse{}, false, fmt.Errorf("token poll request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TokenResponse{}, false, fmt.Errorf("read token poll response: %w", err)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return TokenResponse{}, false, fmt.Errorf("parse token poll response: %w", err)
	}

	if resp.StatusCode >= 400 || tokenResp.Error != "" {
		switch tokenResp.Error {
		case "authorization_pending":
			return TokenResponse{}, true, nil
		case "slow_down":
			return tokenResp, true, nil
		case "":
			return TokenResponse{}, false, fmt.Errorf("token poll failed: endpoint returned %d", resp.StatusCode)
		default:
			msg := tokenResp.Error
			if tokenResp.ErrorDesc != "" {
				msg += ": " + tokenResp.ErrorDesc
			}

			return TokenResponse{}, false, fmt.Errorf("token poll failed: %s", msg)
		}
	}

	return tokenResp, false, nil
}

// AuthorizationFlowHandle holds the state for an in-progress authorization code
// flow started with StartAuthorizationCodeFlow.
type AuthorizationFlowHandle struct {
	cs       *CallbackServer
	pkce     PKCE
	state    string
	tokenURL string
	clientID string
}

// StartAuthorizationCodeFlow begins the authorization code flow by generating
// PKCE parameters, starting a callback server, building the authorization URL,
// and opening the browser. Returns the full authorization URL to display to the
// user and a handle to complete the flow.
func StartAuthorizationCodeFlow(ctx context.Context, authURL, tokenURL, clientID, redirectURI string, scopes []string, extraParams map[string]string) (string, *AuthorizationFlowHandle, error) {
	pkce, err := GeneratePKCE()
	if err != nil {
		return "", nil, fmt.Errorf("generate PKCE: %w", err)
	}

	state, err := GenerateState()
	if err != nil {
		return "", nil, fmt.Errorf("generate state: %w", err)
	}

	cs, err := StartCallbackServer(ctx, redirectURI)
	if err != nil {
		return "", nil, fmt.Errorf("start callback server: %w", err)
	}

	resolvedRedirect := cs.RedirectURI()
	authCodeURL := AuthorizationCodeURL(authURL, clientID, resolvedRedirect, state, pkce, scopes, extraParams)

	if openErr := OpenBrowser(authCodeURL); openErr != nil {
		// Browser open failed (e.g., headless system), but the user can still
		// navigate manually. Log the issue and continue waiting for callback.
		//nolint:sloglint // logging before continuing
		slog.Warn("failed to open browser, please navigate manually", "url", authCodeURL, "error", openErr)
	}

	return authCodeURL, &AuthorizationFlowHandle{
		cs:       cs,
		pkce:     pkce,
		state:    state,
		tokenURL: tokenURL,
		clientID: clientID,
	}, nil
}

// CompleteAuthorizationCodeFlow finishes an authorization code flow that was
// started with StartAuthorizationCodeFlow. It waits for the callback, verifies
// state, exchanges the code for tokens, and returns the credential.
func CompleteAuthorizationCodeFlow(ctx context.Context, handle *AuthorizationFlowHandle) (OAuthCredential, error) {
	defer handle.cs.shutdown()

	// Wait for callback result
	var result CallbackResult
	select {
	case result = <-handle.cs.Result():
		if result.Error != nil {
			return OAuthCredential{}, result.Error
		}
	case <-ctx.Done():
		return OAuthCredential{}, errors.New("oauth flow timed out or was canceled")
	}

	// Verify state matches
	if result.State != handle.state {
		return OAuthCredential{}, errors.New("oauth state mismatch")
	}

	resolvedRedirect := handle.cs.RedirectURI()

	tokenResp, err := ExchangeAuthorizationCode(ctx, handle.tokenURL, handle.clientID, result.Code, resolvedRedirect, handle.pkce)
	if err != nil {
		return OAuthCredential{}, fmt.Errorf("exchange code: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return OAuthCredential{}, errors.New("token exchange response missing access_token")
	}

	cred := OAuthCredential{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
	}

	if tokenResp.ExpiresIn > 0 {
		cred.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	return cred, nil
}

// RunAuthorizationCodeFlow executes the full OAuth authorization code flow.
// It starts a callback server, opens the browser, waits for the callback,
// exchanges the code for tokens, and returns the credential.
func RunAuthorizationCodeFlow(ctx context.Context, authURL, tokenURL, clientID, redirectURI string, scopes []string, extraParams map[string]string) (OAuthCredential, error) {
	_, handle, err := StartAuthorizationCodeFlow(ctx, authURL, tokenURL, clientID, redirectURI, scopes, extraParams)
	if err != nil {
		return OAuthCredential{}, err
	}

	return CompleteAuthorizationCodeFlow(ctx, handle)
}
