package jsonlstore

import (
	"encoding/json"
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

func (s *Store) sessionDir() (string, error) {
	if s.cfg.Dir != "" {
		return s.cfg.Dir, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".weave", "sessions"), nil
}
