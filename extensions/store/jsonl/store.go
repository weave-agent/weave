package jsonlstore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

func (s *Store) Subscribe(bus sdk.Bus) {}

func (s *Store) Close() error { return nil }

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
