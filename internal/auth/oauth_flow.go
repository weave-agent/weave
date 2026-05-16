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
func AuthorizationCodeURL(authURL, clientID, redirectURI, state string, pkce PKCE, scopes []string) string {
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

		return TokenResponse{}, fmt.Errorf("token exchange failed: %s", msg)
	}

	return tokenResp, nil
}

// OAuthFlowResult holds the outcome of an authorization code flow.
type OAuthFlowResult struct {
	Credential OAuthCredential
	Error      error
}

// RunAuthorizationCodeFlow executes the full OAuth authorization code flow.
// It starts a callback server, opens the browser, waits for the callback,
// exchanges the code for tokens, and returns the credential.
func RunAuthorizationCodeFlow(ctx context.Context, authURL, tokenURL, clientID string, scopes []string) (OAuthCredential, error) {
	pkce, err := GeneratePKCE()
	if err != nil {
		return OAuthCredential{}, fmt.Errorf("generate PKCE: %w", err)
	}

	state, err := GenerateState()
	if err != nil {
		return OAuthCredential{}, fmt.Errorf("generate state: %w", err)
	}

	cs, err := StartCallbackServer(ctx)
	if err != nil {
		return OAuthCredential{}, fmt.Errorf("start callback server: %w", err)
	}

	redirectURI := cs.RedirectURI()
	authCodeURL := AuthorizationCodeURL(authURL, clientID, redirectURI, state, pkce, scopes)

	if openErr := OpenBrowser(authCodeURL); openErr != nil {
		return OAuthCredential{}, fmt.Errorf("open browser: %w", openErr)
	}

	// Wait for callback result
	var result CallbackResult
	select {
	case result = <-cs.Result():
		if result.Error != nil {
			return OAuthCredential{}, result.Error
		}
	case <-ctx.Done():
		return OAuthCredential{}, errors.New("oauth flow timed out or was canceled")
	}

	// Verify state matches
	if result.State != state {
		return OAuthCredential{}, errors.New("oauth state mismatch")
	}

	tokenResp, err := ExchangeAuthorizationCode(ctx, tokenURL, clientID, result.Code, redirectURI, pkce)
	if err != nil {
		return OAuthCredential{}, fmt.Errorf("exchange code: %w", err)
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
