package jsonlstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nniel-ape/gonfig"

	"weave/sdk"
)

type SessionHeader struct {
	Type      string    `json:"type"`
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	CWD       string    `json:"cwd"`
}

type Entry struct {
	ID       string          `json:"id"`
	ParentID string          `json:"parent_id,omitempty"`
	Type     string          `json:"type"`
	Turn     int             `json:"turn"`
	Data     json.RawMessage `json:"data"`
	Created  time.Time       `json:"created"`
}

type Session struct {
	Header  SessionHeader
	Entries []Entry
}

type SessionInfo struct {
	ID         string    `json:"id"`
	CWD        string    `json:"cwd"`
	EntryCount int       `json:"entry_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type JSONLOpts struct {
	Dir string `default:"" description:"Session directory (default: ~/.weave/sessions)"`
}

type Config struct {
	JSONL JSONLOpts
}

type Store struct {
	cfg Config

	mu        sync.Mutex
	sessionID string
	lastEntry string
	turn      int
	cancel    context.CancelFunc
	done      chan struct{}
}

func init() { //nolint:gochecknoinits // required for extension self-registration
	sdk.RegisterExtension("jsonl", func(cfg sdk.Config) (sdk.Extension, error) {
		return NewStore(cfg)
	})
}

func NewStore(cfg sdk.Config) (*Store, error) {
	var c Config

	opts := []gonfig.Option{gonfig.WithEnvPrefix("WEAVE")}
	if cfg != nil && cfg.FilePath() != "" {
		opts = append(opts, gonfig.WithFile(cfg.FilePath()))
	}

	if err := gonfig.Load(&c, opts...); err != nil {
		return nil, fmt.Errorf("jsonl config: %w", err)
	}

	return &Store{cfg: c}, nil
}

func (s *Store) Name() string { return "jsonl" }

func (s *Store) Subscribe(bus sdk.Bus) {
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		panic("jsonl: Subscribe called twice without Close")
	}

	ch := bus.Subscribe(
		"agent.prompt",
		"agent.followup",
		"agent.steer",
		"agent.turn_start",
		"agent.message_end",
		"agent.tool_result",
		"agent.end",
	)

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	s.mu.Unlock()

	go s.run(ctx, ch)
}

func (s *Store) Close() error {
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if done != nil {
		<-done
	}

	return nil
}

func (s *Store) run(ctx context.Context, ch <-chan sdk.Event) {
	defer close(s.done)

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}

			switch evt.Topic {
			case "agent.prompt":
				s.handlePrompt(evt)
			case "agent.followup":
				s.handleFollowup(evt)
			case "agent.steer":
				s.handleSteer(evt)
			case "agent.turn_start":
				s.handleTurnStart(evt)
			case "agent.message_end":
				s.handleMsgEnd(evt)
			case "agent.tool_result":
				s.handleToolResult(evt)
			case "agent.end":
				return
			}
		}
	}
}

func (s *Store) handlePrompt(evt sdk.Event) {
	cwd, _ := os.Getwd()
	if cwd == "" {
		cwd = "/"
	}

	sess, err := s.Create(cwd)
	if err != nil {
		slog.Error("jsonl: create session", "error", err)
		return
	}

	data, _ := json.Marshal(map[string]any{
		"role":    sdk.RoleUser,
		"content": evt.Payload,
	})

	entry := Entry{
		Type: "message",
		Turn: 1,
		Data: data,
	}

	entryID, err := s.Append(sess.Header.ID, entry)
	if err != nil {
		slog.Error("jsonl: append entry", "error", err)
		return
	}

	s.mu.Lock()
	s.sessionID = sess.Header.ID
	s.lastEntry = entryID
	s.turn = 1
	s.mu.Unlock()
}

func (s *Store) handleFollowup(evt sdk.Event) {
	s.mu.Lock()
	sessionID := s.sessionID
	lastEntry := s.lastEntry
	turn := s.turn + 1
	s.mu.Unlock()

	s.appendUserEntry(sessionID, turn, lastEntry, evt)
}

func (s *Store) handleSteer(evt sdk.Event) {
	s.mu.Lock()
	sessionID := s.sessionID
	lastEntry := s.lastEntry
	turn := s.turn
	s.mu.Unlock()

	s.appendUserEntry(sessionID, turn, lastEntry, evt)
}

func (s *Store) appendUserEntry(sessionID string, turn int, parentID string, evt sdk.Event) {
	if sessionID == "" {
		return
	}

	data, _ := json.Marshal(map[string]any{
		"role":    sdk.RoleUser,
		"content": evt.Payload,
	})

	entry := Entry{
		ParentID: parentID,
		Type:     "message",
		Turn:     turn,
		Data:     data,
	}

	entryID, err := s.Append(sessionID, entry)
	if err != nil {
		slog.Error("jsonl: append user input", "error", err)
		return
	}

	s.mu.Lock()
	s.lastEntry = entryID
	s.mu.Unlock()
}

func (s *Store) handleTurnStart(evt sdk.Event) {
	turn, ok := evt.Payload.(int)
	if !ok {
		return
	}

	s.mu.Lock()
	s.turn = turn
	s.mu.Unlock()
}

func (s *Store) handleMsgEnd(evt sdk.Event) {
	s.mu.Lock()
	sessionID := s.sessionID
	lastEntry := s.lastEntry
	turn := s.turn
	s.mu.Unlock()

	if sessionID == "" {
		return
	}

	id, err := generateID()
	if err != nil {
		return
	}

	payload := map[string]any{
		"role": sdk.RoleAssistant,
	}

	switch p := evt.Payload.(type) {
	case map[string]any:
		if c, ok := p["content"]; ok {
			payload["content"] = c
		}

		if tc, ok := p["tool_calls"]; ok {
			payload["tool_calls"] = tc
		}
	case string:
		payload["content"] = p
	}

	data, _ := json.Marshal(payload)

	entry := Entry{
		ID:       id,
		ParentID: lastEntry,
		Type:     "message",
		Turn:     turn,
		Data:     data,
	}

	if _, err := s.Append(sessionID, entry); err != nil {
		slog.Error("jsonl: append entry", "error", err)
		return
	}

	s.mu.Lock()
	s.lastEntry = id
	s.mu.Unlock()
}

func (s *Store) handleToolResult(evt sdk.Event) {
	s.mu.Lock()
	sessionID := s.sessionID
	lastEntry := s.lastEntry
	turn := s.turn
	s.mu.Unlock()

	if sessionID == "" {
		return
	}

	id, err := generateID()
	if err != nil {
		return
	}

	data, _ := json.Marshal(map[string]any{
		"role": sdk.RoleToolResult,
		"tool": evt.Payload,
	})

	entry := Entry{
		ID:       id,
		ParentID: lastEntry,
		Type:     "message",
		Turn:     turn,
		Data:     data,
	}

	if _, err := s.Append(sessionID, entry); err != nil {
		slog.Error("jsonl: append entry", "error", err)
		return
	}

	s.mu.Lock()
	s.lastEntry = id
	s.mu.Unlock()
}

func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}

	return hex.EncodeToString(b), nil
}

func (s *Store) sessionDir() (string, error) {
	if s.cfg.JSONL.Dir != "" {
		return s.cfg.JSONL.Dir, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("session dir: %w", err)
	}

	return filepath.Join(home, ".weave", "sessions"), nil
}

func (s *Store) sessionPath(sessionID string) (string, error) {
	if !isValidID(sessionID) {
		return "", fmt.Errorf("invalid session ID: %q", sessionID)
	}

	dir, err := s.sessionDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, sessionID+".jsonl"), nil
}

func isValidID(id string) bool {
	if id == "" {
		return false
	}

	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}

	return true
}

func (s *Store) Create(cwd string) (*Session, error) {
	id, err := generateID()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	sess := &Session{
		Header: SessionHeader{
			Type:      "session",
			ID:        id,
			Timestamp: now,
			CWD:       cwd,
		},
	}

	dir, err := s.sessionDir()
	if err != nil {
		return nil, err
	}

	if err = os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}

	line, err := json.Marshal(sess.Header)
	if err != nil {
		return nil, fmt.Errorf("marshal header: %w", err)
	}

	path := filepath.Join(dir, id+".jsonl")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create session file: %w", err)
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, "%s\n", line); err != nil {
		return nil, fmt.Errorf("write header: %w", err)
	}

	return sess, nil
}

func (s *Store) Append(sessionID string, entry Entry) (string, error) {
	if entry.ID == "" {
		id, err := generateID()
		if err != nil {
			return "", err
		}

		entry.ID = id
	}

	if entry.Created.IsZero() {
		entry.Created = time.Now().UTC()
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return "", fmt.Errorf("marshal entry: %w", err)
	}

	path, err := s.sessionPath(sessionID)
	if err != nil {
		return "", err
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return "", fmt.Errorf("open session file: %w", err)
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, "%s\n", line); err != nil {
		return "", fmt.Errorf("append entry: %w", err)
	}

	return entry.ID, nil
}

func loadFromFile(path string) (*Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read session file: %w", err)
	}

	lines := splitLines(data)
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty session file: %s", path)
	}

	var header SessionHeader
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		return nil, fmt.Errorf("parse header: %w", err)
	}

	entries := make([]Entry, 0, len(lines)-1)
	for _, line := range lines[1:] {
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("parse entry: %w", err)
		}

		entries = append(entries, entry)
	}

	return &Session{Header: header, Entries: entries}, nil
}

func (s *Store) Load(sessionID string) (*Session, error) {
	path, err := s.sessionPath(sessionID)
	if err != nil {
		return nil, err
	}

	return loadFromFile(path)
}

func (s *Store) History(sessionID string) ([]Entry, error) {
	sess, err := s.Load(sessionID)
	if err != nil {
		return nil, err
	}

	return sess.Entries, nil
}

func (s *Store) List() ([]SessionInfo, error) {
	dir, err := s.sessionDir()
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

	var infos []SessionInfo

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}

		path := filepath.Join(dir, e.Name())

		sess, err := loadFromFile(path)
		if err != nil {
			continue
		}

		fi, err := e.Info()
		if err != nil {
			continue
		}

		infos = append(infos, SessionInfo{
			ID:         sess.Header.ID,
			CWD:        sess.Header.CWD,
			EntryCount: len(sess.Entries),
			CreatedAt:  sess.Header.Timestamp,
			UpdatedAt:  fi.ModTime().UTC(),
		})
	}

	return infos, nil
}

func (s *Store) Compact(sessionID string, keepLast int) error {
	sess, err := s.Load(sessionID)
	if err != nil {
		return fmt.Errorf("compact load: %w", err)
	}

	if keepLast >= len(sess.Entries) {
		return nil
	}

	removed := sess.Entries[:len(sess.Entries)-keepLast]
	kept := sess.Entries[len(sess.Entries)-keepLast:]

	summaryID, err := generateID()
	if err != nil {
		return err
	}

	summary := Entry{
		ID:      summaryID,
		Type:    "summary",
		Turn:    removed[len(removed)-1].Turn,
		Data:    json.RawMessage(fmt.Sprintf(`{"removed_count":%d,"first_turn":%d,"last_turn":%d}`, len(removed), removed[0].Turn, removed[len(removed)-1].Turn)),
		Created: time.Now().UTC(),
	}

	if len(kept) > 0 {
		kept[0].ParentID = summary.ID
	}

	newEntries := make([]Entry, 0, 1+len(kept))
	newEntries = append(newEntries, summary)
	newEntries = append(newEntries, kept...)

	return s.rewriteFile(sessionID, sess.Header, newEntries)
}

func (s *Store) rewriteFile(sessionID string, header SessionHeader, entries []Entry) error {
	path, err := s.sessionPath(sessionID)
	if err != nil {
		return err
	}

	headerLine, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("marshal header: %w", err)
	}

	var lines []string

	lines = append(lines, string(headerLine))

	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("marshal entry: %w", err)
		}

		lines = append(lines, string(line))
	}

	var buf []byte
	for _, l := range lines {
		buf = append(buf, l...)
		buf = append(buf, '\n')
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, buf, 0o600); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

func splitLines(data []byte) []string {
	var lines []string

	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		if len(line) > 0 {
			lines = append(lines, string(line))
		}
	}

	return lines
}
