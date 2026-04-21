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

	tea "github.com/charmbracelet/bubbletea"
)

// SessionEntry holds minimal session metadata for the selector.
type SessionEntry struct {
	ID        string
	CWD       string
	CreatedAt time.Time
}

// sessionDir returns the directory where session JSONL files are stored.
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

// sessionHeader matches the first-line JSON of each JSONL session file.
type sessionHeader struct {
	Type      string    `json:"type"`
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	CWD       string    `json:"cwd"`
}

// listSessions reads session headers from the session directory.
// Returns sessions sorted by most recent first.
func listSessions() ([]SessionEntry, error) {
	dir, err := sessionDir()
	if err != nil {
		return nil, err
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

		sessions = append(sessions, SessionEntry{
			ID:        header.ID,
			CWD:       header.CWD,
			CreatedAt: header.Timestamp,
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].CreatedAt.After(sessions[j].CreatedAt)
	})

	return sessions, nil
}

func readSessionHeader(path string) (*sessionHeader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
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
	Role    string `json:"role"`
	Content string `json:"content"`
}

// loadSessionEntries reads all message entries from a session file.
func loadSessionEntries(sessionID string) ([]sessionEntryData, error) {
	dir, err := sessionDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, sessionID+".jsonl")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read session: %w", err)
	}

	lines := splitSessionLines(data)
	if len(lines) <= 1 {
		return nil, nil
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

		if raw.Type != "message" {
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
func listSessionsCmd() tea.Cmd {
	return func() tea.Msg {
		sessions, err := listSessions()
		return SessionListResultMsg{Sessions: sessions, Err: err}
	}
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
