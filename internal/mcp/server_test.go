package mcp

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Guuzzeji/ai-shared-memory/internal/chunking"
	"github.com/Guuzzeji/ai-shared-memory/internal/config"
	"github.com/Guuzzeji/ai-shared-memory/internal/docs"
	"github.com/Guuzzeji/ai-shared-memory/internal/index"
	"github.com/Guuzzeji/ai-shared-memory/internal/logstore"
	"github.com/Guuzzeji/ai-shared-memory/internal/sync"
	"github.com/Guuzzeji/ai-shared-memory/internal/taxonomy"
)

type fakeEmbedder struct {
	dims     int
	lastText string
}

func (f *fakeEmbedder) Embed(text string) ([]float32, error) {
	f.lastText = text
	v := make([]float32, f.dims)
	for i := range v {
		v[i] = 1 / float32(math.Sqrt(float64(f.dims)))
	}
	return v, nil
}

func (f *fakeEmbedder) CountTokens(text string) int { return len(text) / 4 }

const testConfig = `system:
  log_file: .mnemo/memory_log.jsonl
  docs_dir: .mnemo/docs
database:
  path: .mnemo/index.db
embeddings:
  model_repo: onnx-community/embeddinggemma-300m-ONNX
  model_path: .mnemo/models/embeddinggemma-300m.onnx
  dimensions: 768
  search_top_k: 5
  chunking:
    split_on: ["## "]
    max_chunk_tokens: 500
    overlap_tokens: 50
taxonomy:
  allowed_categories: [backend, frontend, devops, architecture]
  key_terms:
    authentication: [auth, jwt, session, login]
    database: [db, sql, sqlite, schema]
`

type testEnv struct {
	cfg   *config.Config
	log   *logstore.Store
	docs  *docs.Store
	idx   *index.Index
	tax   *taxonomy.Taxonomy
	embed *fakeEmbedder
	h     *server
}

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".memory_config.yaml")
	if err := os.WriteFile(cfgPath, []byte(testConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".mnemo/docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	log, err := logstore.New(filepath.Join(dir, ".mnemo/memory_log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	docsStore, err := docs.New(filepath.Join(dir, ".mnemo/docs"), cfg.Taxonomy.AllowedCategories)
	if err != nil {
		t.Fatal(err)
	}
	embed := &fakeEmbedder{dims: cfg.Embeddings.Dimensions}
	chunker := chunking.New(cfg.Embeddings.Chunking.SplitOn, cfg.Embeddings.Chunking.MaxChunkTokens, cfg.Embeddings.Chunking.OverlapTokens, embed)
	idx, err := index.Open(filepath.Join(dir, ".mnemo/index.db"), cfg.Embeddings.Dimensions)
	if err != nil {
		t.Fatal(err)
	}
	syncer := &sync.Syncer{
		Log:      log,
		Docs:     docsStore,
		Chunker:  chunker,
		Embedder: embed,
		Index:    idx,
		ModelID:  "test-768",
	}
	tax := taxonomy.New(cfg.Taxonomy.AllowedCategories, cfg.Taxonomy.KeyTerms)
	return &testEnv{
		cfg:   cfg,
		log:   log,
		docs:  docsStore,
		idx:   idx,
		tax:   tax,
		embed: embed,
		h:     &server{deps: Deps{Config: cfg, Log: log, Docs: docsStore, Index: idx, Syncer: syncer, Taxonomy: tax, Embedder: embed}},
	}
}

func (e *testEnv) addDoc(t *testing.T, id string, tags []string, status string) {
	t.Helper()
	content := "---\nid: " + id + "\ntags: [" + strings.Join(tags, ", ") + "]\nstatus: " + status + "\n---\n## Summary\n" + id + " content here.\n\n## Key Decisions\nUse " + id + ".\n"
	if _, err := e.docs.Create(docs.Doc{ID: id}, []byte(content)); err != nil {
		t.Fatal(err)
	}
	entry := logstore.MemoryLogEntry{
		LogID:      "01900000-0000-7000-8000-00000000000" + id[:1],
		Timestamp:  mustTime(t),
		Author:     "test",
		Action:     logstore.ActionAdded,
		TargetDocs: []string{id},
		Tags:       tags,
		Summary:    "add " + id,
	}
	if err := e.log.Append(&entry); err != nil {
		t.Fatal(err)
	}
	if err := e.h.deps.Syncer.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func mustTime(t *testing.T) (ts time.Time) {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, "2026-08-15T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func TestSemanticSearchBasic(t *testing.T) {
	e := newEnv(t)
	e.addDoc(t, "auth-flow", []string{"backend"}, "active")

	_, out, err := e.h.semanticSearch(context.Background(), nil, semanticSearchInput{Query: "auth-flow content here"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Results) == 0 {
		t.Fatal("want results, got none")
	}
	if out.Results[0].DocID != "auth-flow" {
		t.Fatalf("want auth-flow, got %s", out.Results[0].DocID)
	}
}

func TestSemanticSearchTagsFilter(t *testing.T) {
	e := newEnv(t)
	e.addDoc(t, "auth-flow", []string{"backend"}, "active")

	_, out, err := e.h.semanticSearch(context.Background(), nil, semanticSearchInput{Query: "x", Tags: []string{"backend"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Results) == 0 {
		t.Fatal("want results with backend tag")
	}

	_, out, err = e.h.semanticSearch(context.Background(), nil, semanticSearchInput{Query: "x", Tags: []string{"frontend"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 0 {
		t.Fatalf("want no results with frontend tag, got %d", len(out.Results))
	}
}

func TestSemanticSearchExcludesDeprecated(t *testing.T) {
	e := newEnv(t)
	e.addDoc(t, "old-doc", []string{"backend"}, "deprecated")

	_, out, err := e.h.semanticSearch(context.Background(), nil, semanticSearchInput{Query: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 0 {
		t.Fatalf("want no results for deprecated doc, got %d", len(out.Results))
	}
}

func TestSemanticSearchAliasExpansion(t *testing.T) {
	e := newEnv(t)
	e.addDoc(t, "auth-flow", []string{"backend"}, "active")

	_, _, err := e.h.semanticSearch(context.Background(), nil, semanticSearchInput{Query: "auth"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.embed.lastText, "authentication") {
		t.Fatalf("query should be expanded through key_terms, got: %q", e.embed.lastText)
	}
}

func TestSemanticSearchDegradedMode(t *testing.T) {
	e := newEnv(t)
	e.h.deps.Embedder = nil

	_, _, err := e.h.semanticSearch(context.Background(), nil, semanticSearchInput{Query: "x"})
	if err == nil || !strings.Contains(err.Error(), "index unavailable") {
		t.Fatalf("want index unavailable error, got: %v", err)
	}
}

func TestAppendMemoryCreate(t *testing.T) {
	e := newEnv(t)
	in := appendMemoryInput{
		Entry: struct {
			Action     string   `json:"action" jsonschema:"required"`
			TargetDocs []string `json:"target_docs" jsonschema:"required"`
			Tags       []string `json:"tags" jsonschema:"required"`
			Summary    string   `json:"summary" jsonschema:"required"`
			Reasoning  string   `json:"reasoning,omitempty"`
		}{Action: "ADDED", TargetDocs: []string{"new-doc"}, Tags: []string{"backend"}, Summary: "add new doc"},
		MarkdownUpdates: []markdownPatch{{
			DocID: "new-doc",
			Op:    "create",
			Frontmatter: map[string]any{
				"id":     "new-doc",
				"tags":   []string{"backend"},
				"status": "active",
			},
			Body: "## Summary\nNew doc body.\n\n## Key Decisions\nDecision here.\n",
		}},
	}
	_, out, err := e.h.appendMemory(context.Background(), nil, in)
	if err != nil {
		t.Fatal(err)
	}
	if out.LogID == "" {
		t.Fatal("log_id should be server-set")
	}
	entries, err := e.log.ReadAfter("")
	if err != nil || len(entries) != 1 {
		t.Fatalf("want 1 log entry, got %d (%v)", len(entries), err)
	}
	if entries[0].Author == "" || entries[0].Timestamp.IsZero() {
		t.Fatal("author and timestamp should be server-set")
	}
	if !e.docs.Exists("new-doc") {
		t.Fatal("doc should exist on disk")
	}
	docs, err := e.idx.ListDocs()
	if err != nil || len(docs) != 1 || docs[0].ID != "new-doc" {
		t.Fatalf("doc should be indexed, got %+v (%v)", docs, err)
	}
}

func TestAppendMemoryAppendSection(t *testing.T) {
	e := newEnv(t)
	e.addDoc(t, "auth-flow", []string{"backend"}, "active")

	in := appendMemoryInput{
		Entry: struct {
			Action     string   `json:"action" jsonschema:"required"`
			TargetDocs []string `json:"target_docs" jsonschema:"required"`
			Tags       []string `json:"tags" jsonschema:"required"`
			Summary    string   `json:"summary" jsonschema:"required"`
			Reasoning  string   `json:"reasoning,omitempty"`
		}{Action: "CHANGED", TargetDocs: []string{"auth-flow"}, Tags: []string{"backend"}, Summary: "append section"},
		MarkdownUpdates: []markdownPatch{{
			DocID:   "auth-flow",
			Op:      "append_section",
			Heading: "New Section",
			Content: "Extra content.",
		}},
	}
	_, _, err := e.h.appendMemory(context.Background(), nil, in)
	if err != nil {
		t.Fatal(err)
	}
	content, err := e.docs.Get("auth-flow")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "## New Section") {
		t.Fatal("section should be appended")
	}
}

func TestAppendMemoryRejectsUnknownTag(t *testing.T) {
	e := newEnv(t)
	in := appendMemoryInput{
		Entry: struct {
			Action     string   `json:"action" jsonschema:"required"`
			TargetDocs []string `json:"target_docs" jsonschema:"required"`
			Tags       []string `json:"tags" jsonschema:"required"`
			Summary    string   `json:"summary" jsonschema:"required"`
			Reasoning  string   `json:"reasoning,omitempty"`
		}{Action: "ADDED", TargetDocs: []string{"x"}, Tags: []string{"bogus"}, Summary: "s"},
	}
	_, _, err := e.h.appendMemory(context.Background(), nil, in)
	if err == nil {
		t.Fatal("want error for unknown tag")
	}
	if !strings.Contains(err.Error(), "backend") {
		t.Fatalf("error should list valid categories, got: %v", err)
	}
}

func TestAppendMemoryDeprecation(t *testing.T) {
	e := newEnv(t)
	e.addDoc(t, "auth-flow", []string{"backend"}, "active")

	in := appendMemoryInput{
		Entry: struct {
			Action     string   `json:"action" jsonschema:"required"`
			TargetDocs []string `json:"target_docs" jsonschema:"required"`
			Tags       []string `json:"tags" jsonschema:"required"`
			Summary    string   `json:"summary" jsonschema:"required"`
			Reasoning  string   `json:"reasoning,omitempty"`
		}{Action: "DEPRECATED", TargetDocs: []string{"auth-flow"}, Tags: []string{"backend"}, Summary: "deprecate"},
	}
	_, _, err := e.h.appendMemory(context.Background(), nil, in)
	if err != nil {
		t.Fatal(err)
	}
	content, err := e.docs.Get("auth-flow")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "status: deprecated") {
		t.Fatal("doc frontmatter should be deprecated")
	}
	_, out, err := e.h.semanticSearch(context.Background(), nil, semanticSearchInput{Query: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 0 {
		t.Fatalf("deprecated doc should be invisible to search, got %d results", len(out.Results))
	}
}

func TestAppendMemoryDegradedMode(t *testing.T) {
	e := newEnv(t)
	e.h.deps.Embedder = nil
	e.h.deps.Syncer = nil

	in := appendMemoryInput{
		Entry: struct {
			Action     string   `json:"action" jsonschema:"required"`
			TargetDocs []string `json:"target_docs" jsonschema:"required"`
			Tags       []string `json:"tags" jsonschema:"required"`
			Summary    string   `json:"summary" jsonschema:"required"`
			Reasoning  string   `json:"reasoning,omitempty"`
		}{Action: "ADDED", TargetDocs: []string{"new-doc"}, Tags: []string{"backend"}, Summary: "s"},
		MarkdownUpdates: []markdownPatch{{
			DocID: "new-doc",
			Op:    "create",
			Frontmatter: map[string]any{
				"id":     "new-doc",
				"tags":   []string{"backend"},
				"status": "active",
			},
			Body: "## Summary\nBody.\n\n## Key Decisions\nDecision.\n",
		}},
	}
	_, _, err := e.h.appendMemory(context.Background(), nil, in)
	if err != nil {
		t.Fatalf("append_memory should work in degraded mode: %v", err)
	}
	if !e.docs.Exists("new-doc") {
		t.Fatal("doc should still be created")
	}
}

func TestGetMemory(t *testing.T) {
	e := newEnv(t)
	e.addDoc(t, "auth-flow", []string{"backend"}, "active")

	_, out, err := e.h.getMemory(context.Background(), nil, getMemoryInput{DocID: "auth-flow"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Content, "## Summary") {
		t.Fatal("should return full markdown")
	}

	_, _, err = e.h.getMemory(context.Background(), nil, getMemoryInput{DocID: "missing"})
	if err == nil {
		t.Fatal("want error for missing doc")
	}
}

func TestReindexTool(t *testing.T) {
	e := newEnv(t)
	e.addDoc(t, "auth-flow", []string{"backend"}, "active")
	e.addDoc(t, "db-flow", []string{"backend"}, "active")

	_, out, err := e.h.reindexMemory(context.Background(), nil, reindexInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.IndexedDocs != 2 {
		t.Fatalf("want 2 indexed docs, got %d", out.IndexedDocs)
	}
	_, sOut, err := e.h.semanticSearch(context.Background(), nil, semanticSearchInput{Query: "auth-flow content here"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sOut.Results) == 0 {
		t.Fatal("search should still return results after reindex")
	}
}

func TestReindexToolDegradedMode(t *testing.T) {
	e := newEnv(t)
	e.h.deps.Embedder = nil
	e.h.deps.Syncer = nil

	_, _, err := e.h.reindexMemory(context.Background(), nil, reindexInput{})
	if err == nil || !strings.Contains(err.Error(), "index unavailable") {
		t.Fatalf("want index unavailable error, got: %v", err)
	}
}

func TestListMemories(t *testing.T) {
	e := newEnv(t)
	e.addDoc(t, "auth-flow", []string{"backend"}, "active")
	e.addDoc(t, "old-doc", []string{"backend"}, "deprecated")

	_, out, err := e.h.listMemories(context.Background(), nil, listMemoriesInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Memories) != 2 {
		t.Fatalf("want 2 memories, got %d", len(out.Memories))
	}

	_, out, err = e.h.listMemories(context.Background(), nil, listMemoriesInput{Status: "deprecated"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Memories) != 1 || out.Memories[0].DocID != "old-doc" {
		t.Fatalf("want only old-doc, got %+v", out.Memories)
	}

	_, out, err = e.h.listMemories(context.Background(), nil, listMemoriesInput{Tags: []string{"frontend"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Memories) != 0 {
		t.Fatalf("want no frontend memories, got %d", len(out.Memories))
	}
}
