package jsonlstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

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
	Meta     map[string]any  `json:"meta,omitempty"`
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

type Config struct {
	Dir               string `default:"" description:"Session directory (default: ~/.weave/sessions)"`
	AutoCompact       bool   `default:"false" description:"Auto-compact sessions on close"`
	CompactThreshold  int    `default:"100" description:"Entry count threshold for auto-compact"`
}

type Store struct {
	cfg    Config
	cfgDir string

	mu        sync.Mutex
	sessionID string
	lastEntry string
	turn      int
	cancel    context.CancelFunc
	done      chan struct{}
}

func init() { //nolint:gochecknoinits // required for extension self-registration
	sdk.RegisterExtension("jsonl-store", func(cfg sdk.Config) (sdk.Extension, error) {
		return NewStore(cfg)
	})
}

func NewStore(cfg sdk.Config) (*Store, error) {
	dir := ""
	if cfg != nil {
		dir = cfg.FilePath()
	}

	return &Store{cfgDir: dir}, nil
}

func (s *Store) Name() string { return "jsonl-store" }

func (s *Store) Subscribe(bus sdk.Bus) {
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		panic("jsonl-store: Subscribe called twice without Close")
	}

	promptCh := bus.Subscribe("agent.prompt")
	turnStartCh := bus.Subscribe("agent.turn_start")
	msgEndCh := bus.Subscribe("agent.message_end")
	toolResultCh := bus.Subscribe("agent.tool_result")
	endCh := bus.Subscribe("agent.end")

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	s.mu.Unlock()

	go s.run(ctx, promptCh, turnStartCh, msgEndCh, toolResultCh, endCh)
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

func (s *Store) run(ctx context.Context, promptCh, turnStartCh, msgEndCh, toolResultCh, endCh <-chan sdk.Event) {
	defer close(s.done)

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-promptCh:
			if !ok {
				return
			}
			s.handlePrompt(evt)
		case evt, ok := <-turnStartCh:
			if !ok {
				return
			}
			s.handleTurnStart(evt)
		case evt, ok := <-msgEndCh:
			if !ok {
				return
			}
			s.handleMsgEnd(evt)
		case evt, ok := <-toolResultCh:
			if !ok {
				return
			}
			s.handleToolResult(evt)
		case _, ok := <-endCh:
			if !ok {
				return
			}
			return
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
		return
	}

	data, _ := json.Marshal(map[string]any{
		"role":    sdk.RoleUser,
		"content": evt.Payload,
	})

	entry := Entry{
		Type:    "message",
		Turn:    1,
		Data:    data,
	}

	if err := s.Append(sess.Header.ID, entry); err != nil {
		return
	}

	loaded, err := s.Load(sess.Header.ID)
	if err != nil {
		return
	}

	s.mu.Lock()
	s.sessionID = sess.Header.ID
	if len(loaded.Entries) > 0 {
		s.lastEntry = loaded.Entries[len(loaded.Entries)-1].ID
	}
	s.turn = 1
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

	data, _ := json.Marshal(map[string]any{
		"role":    sdk.RoleAssistant,
		"content": evt.Payload,
	})

	entry := Entry{
		ID:       id,
		ParentID: lastEntry,
		Type:     "message",
		Turn:     turn,
		Data:     data,
	}

	if err := s.Append(sessionID, entry); err != nil {
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

	if err := s.Append(sessionID, entry); err != nil {
		return
	}

	s.mu.Lock()
	s.lastEntry = id
	s.mu.Unlock()
}

func generateID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (s *Store) sessionDir() (string, error) {
	if s.cfg.Dir != "" {
		return s.cfg.Dir, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("session dir: %w", err)
	}

	return filepath.Join(home, ".weave", "sessions"), nil
}

func (s *Store) sessionPath(sessionID string) (string, error) {
	dir, err := s.sessionDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, sessionID+".jsonl"), nil
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}

	line, err := json.Marshal(sess.Header)
	if err != nil {
		return nil, fmt.Errorf("marshal header: %w", err)
	}

	path := filepath.Join(dir, id+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("create session file: %w", err)
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, "%s\n", line); err != nil {
		return nil, fmt.Errorf("write header: %w", err)
	}

	return sess, nil
}

func (s *Store) Append(sessionID string, entry Entry) error {
	if entry.ID == "" {
		id, err := generateID()
		if err != nil {
			return err
		}
		entry.ID = id
	}
	if entry.Created.IsZero() {
		entry.Created = time.Now().UTC()
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}

	path, err := s.sessionPath(sessionID)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open session file: %w", err)
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, "%s\n", line); err != nil {
		return fmt.Errorf("append entry: %w", err)
	}

	return nil
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
		buf = append(buf, []byte(l)...)
		buf = append(buf, '\n')
	}

	return os.WriteFile(path, buf, 0o644)
}

func splitLines(data []byte) []string {
	var lines []string
	for _, line := range splitSlice(data, '\n') {
		if len(line) > 0 {
			lines = append(lines, string(line))
		}
	}
	return lines
}

func splitSlice(data []byte, sep byte) [][]byte {
	var result [][]byte
	for {
		i := indexByte(data, sep)
		if i < 0 {
			if len(data) > 0 {
				result = append(result, data)
			}
			return result
		}
		result = append(result, data[:i])
		data = data[i+1:]
	}
}

func indexByte(data []byte, sep byte) int {
	for i, b := range data {
		if b == sep {
			return i
		}
	}
	return -1
}
