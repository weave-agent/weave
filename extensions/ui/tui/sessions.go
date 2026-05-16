package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

const entryTypeMessage = "message"

// SessionEntry holds minimal session metadata for the selector.
type SessionEntry struct {
	ID        string
	CWD       string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// sessionDir returns the directory where session JSONL files are stored.
// Checks WEAVE_JSONL_DIR env var, then falls back to ~/.weave/sessions.
func sessionDir() (string, error) {
	if dir := os.Getenv("WEAVE_JSONL_DIR"); dir != "" {
		return dir, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("session dir: %w", err)
	}

	return filepath.Join(home, ".weave", "sessions"), nil
}

// resolveSessionDir loads the session directory from config (matching the jsonl store's
// resolution), falling back to env var and default.
func resolveSessionDir(cfgPath string) string {
	dir := resolveSessionDirFromConfig(cfgPath)
	if dir != "" {
		return dir
	}

	dir, err := sessionDir()
	if err != nil {
		return ""
	}

	return dir
}

func resolveSessionDirFromConfig(cfgPath string) string {
	if cfgPath == "" {
		return ""
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return ""
	}

	var raw map[string]any
	if json.Unmarshal(data, &raw) != nil {
		return ""
	}

	jsonl, ok := raw["jsonl"].(map[string]any)
	if !ok {
		return ""
	}

	dir, ok := jsonl["dir"].(string)
	if !ok {
		return ""
	}

	return dir
}

// sessionHeader matches the first-line JSON of each JSONL session file.
type sessionHeader struct {
	Type      string    `json:"type"`
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	CWD       string    `json:"cwd"`
}

// listSessions reads session headers from the session directory.
// Returns sessions sorted by most recent first.
// dirOverride, when non-empty, is used instead of the default session directory.
func listSessions(dirOverride string) ([]SessionEntry, error) {
	dir := dirOverride
	if dir == "" {
		var err error

		dir, err = sessionDir()
		if err != nil {
			return nil, err
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("read session dir: %w", err)
	}

	var sessions []SessionEntry

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}

		path := filepath.Join(dir, e.Name())

		header, err := readSessionHeader(path)
		if err != nil {
			continue
		}

		fi, err := e.Info()
		if err != nil {
			continue
		}

		sessions = append(sessions, SessionEntry{
			ID:        header.ID,
			CWD:       header.CWD,
			CreatedAt: header.Timestamp,
			UpdatedAt: fi.ModTime(),
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

func readSessionHeader(path string) (*sessionHeader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open session header: %w", err)
	}

	defer f.Close()

	dec := json.NewDecoder(f)

	var header sessionHeader

	if err := dec.Decode(&header); err != nil {
		return nil, fmt.Errorf("parse header: %w", err)
	}

	return &header, nil
}

// sessionEntryData is the JSON payload of a message entry.
type sessionEntryData struct {
	Role      string          `json:"role"`
	Content   string          `json:"content"`
	ToolCalls json.RawMessage `json:"tool_calls,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Tool      json.RawMessage `json:"tool,omitempty"`
}

// loadSessionEntries reads all message entries from a session file.
// dirOverride, when non-empty, is used instead of the default session directory.
func loadSessionEntries(dirOverride, sessionID string) ([]sessionEntryData, error) {
	dir := dirOverride
	if dir == "" {
		var err error

		dir, err = sessionDir()
		if err != nil {
			return nil, err
		}
	}

	if !isValidSessionID(sessionID) {
		return nil, fmt.Errorf("invalid session ID: %s", sessionID)
	}

	path := filepath.Join(dir, sessionID+".jsonl")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read session: %w", err)
	}

	lines := splitSessionLines(data)
	if len(lines) <= 1 {
		return []sessionEntryData{}, nil
	}

	var entries []sessionEntryData

	for _, line := range lines[1:] { // skip header
		var raw struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}

		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		if raw.Type != entryTypeMessage {
			continue
		}

		var d sessionEntryData
		if err := json.Unmarshal(raw.Data, &d); err != nil {
			continue
		}

		entries = append(entries, d)
	}

	return entries, nil
}

// shortenCWD replaces the home directory prefix with ~.
func shortenCWD(cwd string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return cwd
	}

	return strings.Replace(cwd, home, "~", 1)
}

// listSessionsCmd returns a tea.Cmd that reads session headers and returns SessionListResultMsg.
func listSessionsCmd(dirOverride string) tea.Cmd {
	return func() tea.Msg {
		sessions, err := listSessions(dirOverride)
		return SessionListResultMsg{Sessions: sessions, Err: err}
	}
}

func isValidSessionID(id string) bool {
	if id == "" {
		return false
	}

	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}

	return true
}

func splitSessionLines(data []byte) []string {
	var lines []string

	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		if len(line) > 0 {
			lines = append(lines, string(line))
		}
	}

	return lines
}
