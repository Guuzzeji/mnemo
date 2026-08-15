package logstore

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ActionType string

const (
	ActionAdded      ActionType = "ADDED"
	ActionChanged    ActionType = "CHANGED"
	ActionDeprecated ActionType = "DEPRECATED"
	ActionFixed      ActionType = "FIXED"
	ActionDecision   ActionType = "DECISION"
)

type MemoryLogEntry struct {
	LogID         string     `json:"log_id"`
	Timestamp     time.Time  `json:"timestamp"`
	Author        string     `json:"author"`
	Action        ActionType `json:"action"`
	TargetDocs    []string   `json:"target_docs"`
	Tags          []string   `json:"tags"`
	Summary       string     `json:"summary"`
	Reasoning     string     `json:"reasoning,omitempty"`
	AffectedFiles []string   `json:"affected_files,omitempty"`
	PRReference   string     `json:"pr_reference,omitempty"`
}

type Store struct {
	path string
	f    *os.File
}

func New(path string) (*Store, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("open logstore: %w", err)
	}
	return &Store{path: path, f: f}, nil
}

func (s *Store) Append(entry *MemoryLogEntry) error {
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("generate uuid: %w", err)
	}
	entry.LogID = id.String()
	entry.Timestamp = time.Now().UTC()
	entry.Author = Author()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}
	data = append(data, '\n')

	if _, err := s.f.Write(data); err != nil {
		return fmt.Errorf("write entry: %w", err)
	}
	if err := s.f.Sync(); err != nil {
		return fmt.Errorf("fsync: %w", err)
	}
	return nil
}

// ReadAfter returns entries whose LogID sorts after the given ID.
// UUIDv7 is time-ordered, so lexicographic string comparison preserves insertion order.
func (s *Store) ReadAfter(logID string) ([]MemoryLogEntry, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return nil, fmt.Errorf("open logstore for read: %w", err)
	}
	defer f.Close()

	var entries []MemoryLogEntry
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry MemoryLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("malformed JSONL at line %d: %w", lineNum, err)
		}
		if logID == "" || entry.LogID > logID {
			entries = append(entries, entry)
		}
	}
	return entries, scanner.Err()
}

func (s *Store) LastLogID() (string, error) {
	entries, err := s.ReadAfter("")
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", nil
	}
	return entries[len(entries)-1].LogID, nil
}

func Author() string {
	if v := os.Getenv("MEMORY_AUTHOR"); v != "" {
		return v
	}
	if name, err := gitUserName(); err == nil && name != "" {
		return name
	}
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "unknown"
}

func gitUserName() (string, error) {
	cmd := exec.Command("git", "config", "user.name")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
