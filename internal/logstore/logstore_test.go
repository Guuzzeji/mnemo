package logstore

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAppendWritesValidJSONLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")
	store, err := New(path)
	if err != nil {
		t.Fatal(err)
	}

	entry := &MemoryLogEntry{
		Action:     ActionAdded,
		TargetDocs: []string{"docs/setup.md"},
		Tags:       []string{"setup"},
		Summary:    "Added setup documentation",
	}

	if err := store.Append(entry); err != nil {
		t.Fatal(err)
	}

	// Read raw file, verify single line is valid JSON
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(string(data))
	if line == "" {
		t.Fatal("file is empty after append")
	}

	var parsed MemoryLogEntry
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		t.Fatalf("appended line is not valid JSON: %v", err)
	}
}

func TestLogIDIsUUIDv7(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")
	store, err := New(path)
	if err != nil {
		t.Fatal(err)
	}

	entry := &MemoryLogEntry{
		Action:  ActionChanged,
		Summary: "Changed something",
	}
	if err := store.Append(entry); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var parsed MemoryLogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &parsed); err != nil {
		t.Fatal(err)
	}

	parsedUUID, err := uuid.Parse(parsed.LogID)
	if err != nil {
		t.Fatalf("LogID is not a valid UUID: %v", err)
	}
	if parsedUUID.Version() != 7 {
		t.Fatalf("LogID version = %d, want 7", parsedUUID.Version())
	}
}

func TestTimestampIsUTC(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")
	store, err := New(path)
	if err != nil {
		t.Fatal(err)
	}

	before := time.Now().UTC()
	entry := &MemoryLogEntry{
		Action:  ActionFixed,
		Summary: "Fixed a bug",
	}
	if err := store.Append(entry); err != nil {
		t.Fatal(err)
	}
	after := time.Now().UTC()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var parsed MemoryLogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &parsed); err != nil {
		t.Fatal(err)
	}

	ts := parsed.Timestamp
	if ts.Location() != time.UTC {
		t.Fatalf("timestamp location = %v, want UTC", ts.Location())
	}
	if ts.Before(before.Add(-time.Second)) || ts.After(after.Add(time.Second)) {
		t.Fatalf("timestamp %v not between %v and %v", ts, before, after)
	}
}

func TestReadAfterReturnsOnlyNewer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")
	store, err := New(path)
	if err != nil {
		t.Fatal(err)
	}

	// Append three entries
	for i, action := range []ActionType{ActionAdded, ActionChanged, ActionDeprecated} {
		e := &MemoryLogEntry{
			Action:  action,
			Summary: strings.Repeat("x", i+1),
		}
		if err := store.Append(e); err != nil {
			t.Fatal(err)
		}
	}

	// Read all
	all, err := store.ReadAfter("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("ReadAfter(\"\") returned %d entries, want 3", len(all))
	}

	// Read after first entry — should get 2
	afterFirst, err := store.ReadAfter(all[0].LogID)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterFirst) != 2 {
		t.Fatalf("ReadAfter(first) returned %d entries, want 2", len(afterFirst))
	}
	if afterFirst[0].LogID != all[1].LogID {
		t.Fatalf("first result LogID = %s, want %s", afterFirst[0].LogID, all[1].LogID)
	}
}

func TestReadAfterEmptyReturnsAll(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")
	store, err := New(path)
	if err != nil {
		t.Fatal(err)
	}

	// Empty file
	all, err := store.ReadAfter("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("ReadAfter(\"\") on empty file returned %d entries, want 0", len(all))
	}

	// One entry
	store.Append(&MemoryLogEntry{Action: ActionDecision, Summary: "decided"})
	all, err = store.ReadAfter("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("ReadAfter(\"\") returned %d entries, want 1", len(all))
	}
}

func TestLastLogIDEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")
	store, err := New(path)
	if err != nil {
		t.Fatal(err)
	}

	id, err := store.LastLogID()
	if err != nil {
		t.Fatal(err)
	}
	if id != "" {
		t.Fatalf("LastLogID on empty file = %q, want \"\"", id)
	}
}

func TestLastLogIDPopulated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")
	store, err := New(path)
	if err != nil {
		t.Fatal(err)
	}

	store.Append(&MemoryLogEntry{Action: ActionAdded, Summary: "first"})
	store.Append(&MemoryLogEntry{Action: ActionChanged, Summary: "second"})

	id, err := store.LastLogID()
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("LastLogID returned empty string on non-empty file")
	}

	// Verify it matches the last entry
	all, _ := store.ReadAfter("")
	if id != all[len(all)-1].LogID {
		t.Fatalf("LastLogID = %s, want %s", id, all[len(all)-1].LogID)
	}
}

func TestAppendContinuesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")

	// First store
	store1, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	store1.Append(&MemoryLogEntry{Action: ActionAdded, Summary: "first"})

	// Second store opens same file
	store2, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	store2.Append(&MemoryLogEntry{Action: ActionChanged, Summary: "second"})

	// Read all — should have 2 entries
	store3, _ := New(path)
	all, err := store3.ReadAfter("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 entries after reopen, got %d", len(all))
	}
	if all[0].Summary != "first" {
		t.Fatalf("first entry summary = %q, want %q", all[0].Summary, "first")
	}
	if all[1].Summary != "second" {
		t.Fatalf("second entry summary = %q, want %q", all[1].Summary, "second")
	}
}

func TestMalformedLineReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")

	// Write valid line + malformed line + valid line
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"log_id":"valid1","action":"ADDED","summary":"ok"}` + "\n")
	f.WriteString(`THIS IS NOT JSON` + "\n")
	f.WriteString(`{"log_id":"valid2","action":"FIXED","summary":"also ok"}` + "\n")
	f.Close()

	store, err := New(path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.ReadAfter("")
	if err == nil {
		t.Fatal("expected error for malformed line, got nil")
	}
	// Error should mention the line number
	if !strings.Contains(err.Error(), "2") {
		t.Fatalf("error should mention line 2, got: %v", err)
	}
}

func TestAuthorEnvOverride(t *testing.T) {
	t.Setenv("MEMORY_AUTHOR", "test-author")
	a := Author()
	if a != "test-author" {
		t.Fatalf("Author() = %q, want %q", a, "test-author")
	}
}

func TestReadAfterLexicographicOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")
	store, err := New(path)
	if err != nil {
		t.Fatal(err)
	}

	// Append several entries
	for i := 0; i < 5; i++ {
		store.Append(&MemoryLogEntry{
			Action:  ActionAdded,
			Summary: strings.Repeat("y", i+1),
		})
	}

	all, _ := store.ReadAfter("")

	// Verify entries are in lexicographic order of LogID
	for i := 1; i < len(all); i++ {
		if all[i].LogID <= all[i-1].LogID {
			t.Fatalf("entries not lexicographically ordered: %s <= %s", all[i].LogID, all[i-1].LogID)
		}
	}

	// ReadAfter with a LogID in the middle — skip count check
	if len(all) < 3 {
		t.Skip("need at least 3 entries")
	}
	midID := all[2].LogID
	result, err := store.ReadAfter(midID)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("ReadAfter(mid) returned %d entries, want 2", len(result))
	}
}

func TestAuthorGitFallback(t *testing.T) {
	// Clear env override
	os.Unsetenv("MEMORY_AUTHOR")

	a := Author()
	if a == "" {
		t.Fatal("Author() returned empty string")
	}
	// Should be a non-empty string (git config user.name or "unknown")
}

// helper to count lines
func countLines(t *testing.T, path string) int {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		count++
	}
	return count
}
