package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"errors"

	"github.com/Guuzzeji/ai-shared-memory/internal/embeddings"
	"github.com/Guuzzeji/ai-shared-memory/internal/index"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeEmbedder struct{ dims int }

func (f *fakeEmbedder) Embed(text string) ([]float32, error) {
	h := sha256.Sum256([]byte(text))
	v := make([]float32, f.dims)
	for i := range v {
		v[i] = float32(h[i%len(h)]) / 255
	}
	return v, nil
}

func (f *fakeEmbedder) CountTokens(text string) int { return len(text) / 4 }

const fixtureConfig = `system:
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

func fixture(t *testing.T, withDoc bool) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".memory_config.yaml"), []byte(fixtureConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{".mnemo/docs", ".mnemo/models"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"tokenizer.json", "config.json", "model.onnx", "model.onnx_data"} {
		if err := os.WriteFile(filepath.Join(dir, ".mnemo/models", f), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if withDoc {
		entry := `{"log_id":"01900000-0000-7000-8000-000000000001","timestamp":"2026-08-15T00:00:00Z","author":"test","action":"ADDED","target_docs":["auth-flow"],"tags":["backend"],"summary":"add auth flow doc"}`
		if err := os.WriteFile(filepath.Join(dir, ".mnemo/memory_log.jsonl"), []byte(entry+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		doc := `---
id: auth-flow
tags: [backend]
status: active
---
## Summary
Authentication flow for the API.

## Key Decisions
Use JWT.
`
		if err := os.WriteFile(filepath.Join(dir, ".mnemo/docs/auth-flow.md"), []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func useFakeEmbedder(t *testing.T) {
	t.Helper()
	old := newEmbedder
	newEmbedder = func(_ context.Context, _ string, dims int) (embeddings.Embedder, error) {
		return &fakeEmbedder{dims: dims}, nil
	}
	t.Cleanup(func() { newEmbedder = old })
}

func TestRunMissingConfig(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"-config", filepath.Join(t.TempDir(), "nope.yaml")}, &out, &errb)
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	if !strings.Contains(errb.String(), "--init") {
		t.Fatalf("stderr should mention --init, got: %s", errb.String())
	}
}

func TestRunBadYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".memory_config.yaml")
	if err := os.WriteFile(cfgPath, []byte("system: [unclosed"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := run([]string{"-config", cfgPath}, &out, &errb)
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
}

func TestRunReindex(t *testing.T) {
	useFakeEmbedder(t)
	dir := fixture(t, false)
	var out, errb bytes.Buffer
	code := run([]string{"-config", filepath.Join(dir, ".memory_config.yaml"), "-reindex"}, &out, &errb)
	if code != 0 {
		t.Fatalf("want exit 0, got %d: %s", code, errb.String())
	}
	idx, err := index.Open(filepath.Join(dir, ".mnemo/index.db"), 768)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	st, err := idx.GetSyncState()
	if err != nil {
		t.Fatalf("sync_state missing: %v", err)
	}
	if st.ModelID == "" {
		t.Fatal("sync_state model_id not set")
	}
}

func useNoopServe(t *testing.T) {
	t.Helper()
	old := serve
	serve = func(_ context.Context, _ *mcp.Server) error { return nil }
	t.Cleanup(func() { serve = old })
}

func TestRunNormalBoot(t *testing.T) {
	useFakeEmbedder(t)
	useNoopServe(t)
	dir := fixture(t, true)
	var out, errb bytes.Buffer
	code := run([]string{"-config", filepath.Join(dir, ".memory_config.yaml")}, &out, &errb)
	if code != 0 {
		t.Fatalf("want exit 0, got %d: %s", code, errb.String())
	}
	idx, err := index.Open(filepath.Join(dir, ".mnemo/index.db"), 768)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	docs, err := idx.ListDocs()
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].ID != "auth-flow" {
		t.Fatalf("want auth-flow indexed, got %+v", docs)
	}
}

func TestRunDegradedMode(t *testing.T) {
	useNoopServe(t)
	old := newEmbedder
	newEmbedder = func(_ context.Context, _ string, _ int) (embeddings.Embedder, error) {
		return nil, errors.New("model load failed")
	}
	t.Cleanup(func() { newEmbedder = old })
	dir := fixture(t, false)
	var out, errb bytes.Buffer
	code := run([]string{"-config", filepath.Join(dir, ".memory_config.yaml")}, &out, &errb)
	if code != 0 {
		t.Fatalf("want exit 0 in degraded mode, got %d: %s", code, errb.String())
	}
}

func TestRunReindexRequiresModel(t *testing.T) {
	old := newEmbedder
	newEmbedder = func(_ context.Context, _ string, _ int) (embeddings.Embedder, error) {
		return nil, errors.New("model load failed")
	}
	t.Cleanup(func() { newEmbedder = old })
	dir := fixture(t, false)
	var out, errb bytes.Buffer
	code := run([]string{"-config", filepath.Join(dir, ".memory_config.yaml"), "-reindex"}, &out, &errb)
	if code != 1 {
		t.Fatalf("want exit 1 for --reindex without model, got %d", code)
	}
}
