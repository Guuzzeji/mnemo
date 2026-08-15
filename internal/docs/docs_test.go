package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validDoc = `---
id: "auth-middleware"
tags: ["backend", "security"]
status: "active"
---

# Auth Middleware

## Summary
Handles JWT validation for all API routes.

## Key Decisions
- Use symmetric HMAC-SHA256 for token signing
- Tokens expire after 24 hours
`

func TestParseValidDoc(t *testing.T) {
	doc, err := Parse([]byte(validDoc))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}
	if doc.ID != "auth-middleware" {
		t.Errorf("ID = %q, want %q", doc.ID, "auth-middleware")
	}
	if len(doc.Tags) != 2 || doc.Tags[0] != "backend" || doc.Tags[1] != "security" {
		t.Errorf("Tags = %v, want [backend security]", doc.Tags)
	}
	if doc.Status != "active" {
		t.Errorf("Status = %q, want %q", doc.Status, "active")
	}
	if !strings.Contains(doc.Body, "## Summary") {
		t.Error("Body should contain ## Summary")
	}
	if !strings.Contains(doc.Body, "## Key Decisions") {
		t.Error("Body should contain ## Key Decisions")
	}
}

func TestParseMissingID(t *testing.T) {
	content := []byte(`---
tags: ["backend"]
status: "active"
---

# Test

## Summary
Test summary.

## Key Decisions
- Decision 1
`)
	_, err := Parse(content)
	if err == nil {
		t.Fatal("Parse() should reject missing id")
	}
	if !strings.Contains(err.Error(), "id") {
		t.Errorf("error should mention id, got: %v", err)
	}
}

func TestParseEmptyID(t *testing.T) {
	content := []byte(`---
id: ""
tags: ["backend"]
status: "active"
---

# Test

## Summary
Test.

## Key Decisions
- Decision
`)
	_, err := Parse(content)
	if err == nil {
		t.Fatal("Parse() should reject empty id")
	}
}

func TestParseMissingTags(t *testing.T) {
	content := []byte(`---
id: "test-doc"
status: "active"
---

# Test

## Summary
Test.

## Key Decisions
- Decision
`)
	_, err := Parse(content)
	if err == nil {
		t.Fatal("Parse() should reject missing tags")
	}
	if !strings.Contains(err.Error(), "tags") {
		t.Errorf("error should mention tags, got: %v", err)
	}
}

func TestParseEmptyTags(t *testing.T) {
	content := []byte(`---
id: "test-doc"
tags: []
status: "active"
---

# Test

## Summary
Test.

## Key Decisions
- Decision
`)
	_, err := Parse(content)
	if err == nil {
		t.Fatal("Parse() should reject empty tags")
	}
}

func TestParseMissingStatus(t *testing.T) {
	content := []byte(`---
id: "test-doc"
tags: ["backend"]
---

# Test

## Summary
Test.

## Key Decisions
- Decision
`)
	_, err := Parse(content)
	if err == nil {
		t.Fatal("Parse() should reject missing status")
	}
	if !strings.Contains(err.Error(), "status") {
		t.Errorf("error should mention status, got: %v", err)
	}
}

func TestParseInvalidStatus(t *testing.T) {
	content := []byte(`---
id: "test-doc"
tags: ["backend"]
status: "unknown"
---

# Test

## Summary
Test.

## Key Decisions
- Decision
`)
	_, err := Parse(content)
	if err == nil {
		t.Fatal("Parse() should reject invalid status")
	}
	if !strings.Contains(err.Error(), "active") || !strings.Contains(err.Error(), "deprecated") {
		t.Errorf("error should mention active/deprecated, got: %v", err)
	}
}

func TestParseMissingSummary(t *testing.T) {
	content := []byte(`---
id: "test-doc"
tags: ["backend"]
status: "active"
---

# Test

## Key Decisions
- Decision
`)
	_, err := Parse(content)
	if err == nil {
		t.Fatal("Parse() should reject missing ## Summary")
	}
	if !strings.Contains(err.Error(), "Summary") {
		t.Errorf("error should mention Summary, got: %v", err)
	}
}

func TestParseMissingKeyDecisions(t *testing.T) {
	content := []byte(`---
id: "test-doc"
tags: ["backend"]
status: "active"
---

# Test

## Summary
Test summary.
`)
	_, err := Parse(content)
	if err == nil {
		t.Fatal("Parse() should reject missing ## Key Decisions")
	}
	if !strings.Contains(err.Error(), "Key Decisions") {
		t.Errorf("error should mention Key Decisions, got: %v", err)
	}
}

func TestParseNoFrontmatter(t *testing.T) {
	content := []byte(`# Just a heading
No frontmatter here.
`)
	_, err := Parse(content)
	if err == nil {
		t.Fatal("Parse() should reject content without frontmatter")
	}
}

func TestValidateContentValid(t *testing.T) {
	allowed := []string{"backend", "frontend", "security"}
	err := ValidateContent([]byte(validDoc), allowed)
	if err != nil {
		t.Errorf("ValidateContent() unexpected error: %v", err)
	}
}

func TestValidateContentBadTags(t *testing.T) {
	content := []byte(`---
id: "test-doc"
tags: ["backend", "nonexistent-tag"]
status: "active"
---

# Test

## Summary
Test.

## Key Decisions
- Decision
`)
	allowed := []string{"backend", "frontend"}
	err := ValidateContent(content, allowed)
	if err == nil {
		t.Fatal("ValidateContent() should reject tags outside allowedCategories")
	}
	if !strings.Contains(err.Error(), "nonexistent-tag") {
		t.Errorf("error should mention the bad tag, got: %v", err)
	}
}

func TestValidateContentCaseInsensitive(t *testing.T) {
	content := []byte(`---
id: "test-doc"
tags: ["Backend", "FRONTEND"]
status: "active"
---

# Test

## Summary
Test.

## Key Decisions
- Decision
`)
	allowed := []string{"backend", "frontend"}
	err := ValidateContent(content, allowed)
	if err != nil {
		t.Errorf("ValidateContent() should be case-insensitive, got: %v", err)
	}
}

func TestCreateAndGet(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir, []string{"backend", "security"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	path, err := store.Create(Doc{
		ID:     "auth-middleware",
		Tags:   []string{"backend", "security"},
		Status: "active",
	}, []byte(validDoc))
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if path != filepath.Join(dir, "auth-middleware.md") {
		t.Errorf("Create() path = %q, want %q", path, filepath.Join(dir, "auth-middleware.md"))
	}

	got, err := store.Get("auth-middleware")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if !strings.Contains(got, "auth-middleware") {
		t.Error("Get() should return content with doc ID")
	}
	if !strings.Contains(got, "## Summary") {
		t.Error("Get() should return full markdown content")
	}
}

func TestCreateDuplicateID(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir, []string{"backend"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	doc := []byte(`---
id: "my-doc"
tags: ["backend"]
status: "active"
---

# My Doc

## Summary
A doc.

## Key Decisions
- Decision
`)
	_, err = store.Create(Doc{ID: "my-doc", Tags: []string{"backend"}, Status: "active"}, doc)
	if err != nil {
		t.Fatalf("Create() first error: %v", err)
	}

	_, err = store.Create(Doc{ID: "my-doc", Tags: []string{"backend"}, Status: "active"}, doc)
	if err == nil {
		t.Fatal("Create() should reject duplicate ID")
	}
	if !strings.Contains(err.Error(), "my-doc") {
		t.Errorf("error should mention the duplicate ID, got: %v", err)
	}
}

func TestCreateInvalidTags(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir, []string{"backend"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	content := []byte(`---
id: "bad-tags"
tags: ["nonexistent"]
status: "active"
---

# Bad Tags

## Summary
Bad.

## Key Decisions
- Decision
`)
	_, err = store.Create(Doc{ID: "bad-tags", Tags: []string{"nonexistent"}, Status: "active"}, content)
	if err == nil {
		t.Fatal("Create() should reject tags outside allowedCategories")
	}
}

func TestAppendSection(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir, []string{"backend"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	doc := []byte(`---
id: "my-doc"
tags: ["backend"]
status: "active"
---

# My Doc

## Summary
A doc.

## Key Decisions
- Decision
`)
	_, err = store.Create(Doc{ID: "my-doc", Tags: []string{"backend"}, Status: "active"}, doc)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	err = store.AppendSection("my-doc", "Open Questions", "- How to handle token refresh?\n")
	if err != nil {
		t.Fatalf("AppendSection() error: %v", err)
	}

	got, err := store.Get("my-doc")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if !strings.Contains(got, "## Open Questions") {
		t.Error("AppendSection() should add the heading")
	}
	if !strings.Contains(got, "token refresh") {
		t.Error("AppendSection() should add the content")
	}
}

func TestAppendSectionRejectsMissingRequiredSections(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir, []string{"backend"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	content := []byte(`---
id: "minimal-doc"
tags: ["backend"]
status: "active"
---

# Minimal

## Summary
Only summary.
`)
	_, err = store.Create(Doc{ID: "minimal-doc", Tags: []string{"backend"}, Status: "active"}, content)
	if err == nil {
		t.Fatal("Create() should reject doc missing Key Decisions")
	}
}

func TestGetNonexistent(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir, []string{"backend"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	_, err = store.Get("nonexistent")
	if err == nil {
		t.Fatal("Get() should return error for nonexistent doc")
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir, []string{"backend", "security"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	doc1 := []byte(`---
id: "doc-one"
tags: ["backend"]
status: "active"
---

# Doc One

## Summary
First doc.

## Key Decisions
- Decision 1
`)
	doc2 := []byte(`---
id: "doc-two"
tags: ["security"]
status: "deprecated"
---

# Doc Two

## Summary
Second doc.

## Key Decisions
- Decision 2
`)
	_, err = store.Create(Doc{ID: "doc-one", Tags: []string{"backend"}, Status: "active"}, doc1)
	if err != nil {
		t.Fatalf("Create(doc1) error: %v", err)
	}
	_, err = store.Create(Doc{ID: "doc-two", Tags: []string{"security"}, Status: "deprecated"}, doc2)
	if err != nil {
		t.Fatalf("Create(doc2) error: %v", err)
	}

	docs, err := store.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("List() returned %d docs, want 2", len(docs))
	}

	found := map[string]bool{}
	for _, d := range docs {
		found[d.ID] = true
		if d.Path == "" {
			t.Errorf("List() doc %q has empty Path", d.ID)
		}
	}
	if !found["doc-one"] || !found["doc-two"] {
		t.Errorf("List() missing docs: %v", found)
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir, []string{"backend"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if store.Exists("nope") {
		t.Error("Exists() should return false for nonexistent doc")
	}

	doc := []byte(`---
id: "exists-doc"
tags: ["backend"]
status: "active"
---

# Exists

## Summary
Exists.

## Key Decisions
- Decision
`)
	_, err = store.Create(Doc{ID: "exists-doc", Tags: []string{"backend"}, Status: "active"}, doc)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if !store.Exists("exists-doc") {
		t.Error("Exists() should return true for existing doc")
	}
}

func TestNewNonexistentDir(t *testing.T) {
	_, err := New("/nonexistent/path/to/docs", []string{"backend"})
	if err == nil {
		t.Fatal("New() should error on nonexistent directory")
	}
}

func TestCreateBadContentMismatch(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir, []string{"backend"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	content := []byte(`---
id: "other-id"
tags: ["backend"]
status: "active"
---

# Other

## Summary
Other doc.

## Key Decisions
- Decision
`)
	_, err = store.Create(Doc{ID: "good-id", Tags: []string{"backend"}, Status: "active"}, content)
	if err == nil {
		t.Fatal("Create() should reject when Doc.ID != frontmatter id")
	}
}

func TestDemo(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir, []string{"backend", "security"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	doc := []byte(`---
id: "demo-doc"
tags: ["backend"]
status: "active"
---

# Demo

## Summary
Demo doc.

## Key Decisions
- Decision
`)
	path, err := store.Create(Doc{ID: "demo-doc", Tags: []string{"backend"}, Status: "active"}, doc)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if !store.Exists("demo-doc") {
		t.Fatal("Exists() should be true after Create")
	}

	content, err := store.Get("demo-doc")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if !strings.Contains(content, "## Summary") {
		t.Fatal("Get() content missing Summary")
	}

	err = store.AppendSection("demo-doc", "New Section", "- Item\n")
	if err != nil {
		t.Fatalf("AppendSection() error: %v", err)
	}

	docs, err := store.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("List() = %d docs, want 1", len(docs))
	}
	if docs[0].ID != "demo-doc" {
		t.Errorf("List()[0].ID = %q, want demo-doc", docs[0].ID)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("file should exist at %s", path)
	}

	t.Logf("demo OK: path=%s, id=%s, tags=%v, status=%s", path, docs[0].ID, docs[0].Tags, docs[0].Status)
}

func TestRejectsTraversalIDs(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, []string{"backend"})
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("---\nid: ../../evil\ntags: [backend]\nstatus: active\n---\n## Summary\nx\n\n## Key Decisions\ny\n")
	if _, err := s.Create(Doc{ID: "../../evil"}, content); err == nil {
		t.Fatal("want error for traversal id in Create")
	}
	if _, err := s.Get("../../evil"); err == nil {
		t.Fatal("want error for traversal id in Get")
	}
	if err := s.AppendSection("../../evil", "## H", "x"); err == nil {
		t.Fatal("want error for traversal id in AppendSection")
	}
	if err := s.SetStatus("../../evil", "deprecated"); err == nil {
		t.Fatal("want error for traversal id in SetStatus")
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "evil.md")); !os.IsNotExist(err) {
		t.Fatalf("traversal file must not exist, stat err=%v", err)
	}
}
