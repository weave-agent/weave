package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// CallbackResult holds the authorization code or an error from the OAuth callback.
type CallbackResult struct {
	Code  string
	State string
	Error error
}

// CallbackServer is a temporary HTTP server that listens for a single OAuth callback
// on localhost, extracts the authorization code, and shuts down automatically.
type CallbackServer struct {
	server        *http.Server
	listener      net.Listener
	fixedRedirect string // original fixed redirect URI, if provided
	result        chan CallbackResult
	mu            sync.Mutex
	closed        bool
	shutdownOnce  sync.Once
}

// StartCallbackServer creates and starts a temporary HTTP server. If
// fixedRedirectURI is non-empty, the server listens on the host:port from that
// URI and handles the path. Otherwise it uses a random localhost port with
// /callback as the path. The server shuts down after receiving one callback or
// when the context is canceled.
func StartCallbackServer(ctx context.Context, fixedRedirectURI string) (*CallbackServer, error) {
	listenAddr := "127.0.0.1:0"
	callbackPath := "/callback"

	if fixedRedirectURI != "" {
		u, err := url.Parse(fixedRedirectURI)
		if err != nil {
			return nil, fmt.Errorf("parse redirect URI: %w", err)
		}

		listenAddr = u.Host

		callbackPath = u.Path
		if callbackPath == "" {
			callbackPath = "/callback"
		}
	}

	lc := net.ListenConfig{}

	listener, err := lc.Listen(ctx, "tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("create callback listener: %w", err)
	}

	cs := &CallbackServer{
		result:        make(chan CallbackResult, 1),
		listener:      listener,
		fixedRedirect: fixedRedirectURI,
	}

	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, cs.handleCallback)

	cs.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		// Serve returns ErrServerClosed on graceful shutdown, which is expected.
		_ = cs.server.Serve(listener)
	}()

	// Auto-shutdown when context is canceled.
	go func() {
		<-ctx.Done()
		cs.shutdown()
	}()

	return cs, nil
}

// RedirectURI returns the callback URL. If a fixed redirect URI was provided,
// it returns that. Otherwise it derives one from the listener address.
func (cs *CallbackServer) RedirectURI() string {
	if cs.fixedRedirect != "" {
		return cs.fixedRedirect
	}

	return fmt.Sprintf("http://%s/callback", cs.listener.Addr().String())
}

// Result returns a read-only channel that receives the callback result.
// If the context is canceled before a callback arrives, the channel is closed
// without sending a result.
func (cs *CallbackServer) Result() <-chan CallbackResult {
	return cs.result
}

func (cs *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Check for OAuth error response.
	if errMsg := query.Get("error"); errMsg != "" {
		cs.sendResult(CallbackResult{
			Error: fmt.Errorf("oauth error: %s: %s", errMsg, query.Get("error_description")),
		})
		http.Error(w, "OAuth error: "+errMsg, http.StatusBadRequest)

		return
	}

	code := query.Get("code")
	if code == "" {
		cs.sendResult(CallbackResult{
			Error: errors.New("missing authorization code"),
		})
		http.Error(w, "Missing authorization code", http.StatusBadRequest)

		return
	}

	cs.sendResult(CallbackResult{
		Code:  code,
		State: query.Get("state"),
	})

	// Return a simple success page to the browser.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>Authentication Successful</title></head>
<body>
<h1>Authentication successful</h1>
<p>You can close this window and return to the application.</p>
</body>
</html>`))
}

func (cs *CallbackServer) sendResult(result CallbackResult) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.closed {
		return
	}

	cs.result <- result

	close(cs.result)
	cs.closed = true

	go cs.shutdown()
}

func (cs *CallbackServer) shutdown() {
	cs.shutdownOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = cs.server.Shutdown(ctx)

		cs.mu.Lock()
		defer cs.mu.Unlock()

		if !cs.closed {
			close(cs.result)
			cs.closed = true
		}
	})
}
