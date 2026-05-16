package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
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
	server   *http.Server
	listener net.Listener
	result   chan CallbackResult
	mu       sync.Mutex
	closed   bool
}

// StartCallbackServer creates and starts a temporary HTTP server on a random
// localhost port. It handles a single /callback request and returns the result
// via the channel. The server shuts down automatically after receiving the
// callback or after the context is canceled.
func StartCallbackServer(ctx context.Context) (*CallbackServer, error) {
	lc := net.ListenConfig{}

	listener, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("create callback listener: %w", err)
	}

	cs := &CallbackServer{
		result:   make(chan CallbackResult, 1),
		listener: listener,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", cs.handleCallback)

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

// RedirectURI returns the full callback URL that should be passed to the OAuth
// authorization endpoint.
func (cs *CallbackServer) RedirectURI() string {
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = cs.server.Shutdown(ctx)

	cs.mu.Lock()
	defer cs.mu.Unlock()

	if !cs.closed {
		close(cs.result)
		cs.closed = true
	}
}
