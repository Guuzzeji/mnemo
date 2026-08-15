package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Guuzzeji/mnemo/internal/chunking"
	"github.com/Guuzzeji/mnemo/internal/docs"
	"github.com/Guuzzeji/mnemo/internal/index"
	"github.com/Guuzzeji/mnemo/internal/logstore"
)

const testDim = 8
const testModelID = "embeddinggemma-300m-768"

type fakeEmbedder struct {
	dim int
}

func (f *fakeEmbedder) Embed(text string) ([]float32, error) {
	sum := sha256.Sum256([]byte(text))
	v := make([]float32, f.dim)
	var norm float64
	for i := 0; i < f.dim; i++ {
		u := float32(int(sum[i%len(sum)])*256+int(sum[(i+1)%len(sum)])) / 65535.0
		v[i] = u*2 - 1
		norm += float64(v[i] * v[i])
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range v {
			v[i] = float32(float64(v[i]) / norm)
		}
	}
	return v, nil
}

func (f *fakeEmbedder) CountTokens(text string) int {
	if text == "" {
		return 0
	}
	return len(strings.Fields(text))
}

func validDocContent(id, status, summaryExtra string) string {
	return fmt.Sprintf(`---
id: %s
tags:
  - architecture
status: %s
---

## Summary
%s

## Key Decisions
Decision for %s.
`, id, status, summaryExtra, id)
}

func multiSectionDoc(id string, sections []string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`---
id: %s
tags:
  - architecture
status: active
---

## Summary
Summary for %s.

## Key Decisions
Decision for %s.
`, id, id, id))
	for _, s := range sections {
		b.WriteString("\n## ")
		b.WriteString(s)
		b.WriteString("\nContent of ")
		b.WriteString(s)
		b.WriteString(".\n")
	}
	return b.String()
}

func writeDoc(t *testing.T, docsDir, id, content string) {
	t.Helper()
	path := filepath.Join(docsDir, id+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
}

func setup(t *testing.T) (*Syncer, string, *logstore.Store, *docs.Store, *index.Index) {
	t.Helper()
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "memory.jsonl")
	dbPath := filepath.Join(root, "index.db")

	ls, err := logstore.New(logPath)
	if err != nil {
		t.Fatalf("logstore: %v", err)
	}

	ds, err := docs.New(docsDir, []string{"architecture", "api", "ops"})
	if err != nil {
		t.Fatalf("docs: %v", err)
	}

	idx, err := index.Open(dbPath, testDim)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	t.Cleanup(func() { idx.Close() })

	emb := &fakeEmbedder{dim: testDim}
	chunker := chunking.New([]string{"## "}, 500, 0, emb)

	s := &Syncer{
		Log:      ls,
		Docs:     ds,
		Chunker:  chunker,
		Embedder: emb,
		Index:    idx,
		ModelID:  testModelID,
	}
	return s, docsDir, ls, ds, idx
}

func chunkID(docID string, chunkIndex int) string {
	h := sha256.Sum256([]byte(docID + strconv.Itoa(chunkIndex)))
	return hex.EncodeToString(h[:])
}

func appendLog(t *testing.T, ls *logstore.Store, action logstore.ActionType, targets []string, summary string) string {
	t.Helper()
	e := &logstore.MemoryLogEntry{
		Action:     action,
		TargetDocs: targets,
		Tags:       []string{"architecture"},
		Summary:    summary,
	}
	if err := ls.Append(e); err != nil {
		t.Fatalf("append: %v", err)
	}
	return e.LogID
}

func TestFullSyncFromEmpty(t *testing.T) {
	s, docsDir, ls, _, idx := setup(t)
	writeDoc(t, docsDir, "doc1", validDocContent("doc1", "active", "Hello doc1."))
	logID := appendLog(t, ls, logstore.ActionAdded, []string{"doc1"}, "add doc1")

	if err := s.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	doc, err := idx.GetDoc("doc1")
	if err != nil {
		t.Fatalf("GetDoc: %v", err)
	}
	if doc.Status != "active" {
		t.Errorf("status=%q", doc.Status)
	}
	var tags []string
	if err := json.Unmarshal([]byte(doc.TagsJSON), &tags); err != nil {
		t.Fatalf("tags json: %v", err)
	}
	if len(tags) == 0 || tags[0] != "architecture" {
		t.Errorf("tags=%v", tags)
	}

	n, err := idx.ChunkCount("doc1")
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("expected chunks for doc1")
	}

	st, err := idx.GetSyncState()
	if err != nil {
		t.Fatalf("GetSyncState: %v", err)
	}
	if st.LastSyncedLogID != logID {
		t.Errorf("last_synced_log_id=%q want %q", st.LastSyncedLogID, logID)
	}
	if st.ModelID != testModelID {
		t.Errorf("model_id=%q", st.ModelID)
	}
}

func TestDeltaSyncOnlyNewDoc(t *testing.T) {
	s, docsDir, ls, _, idx := setup(t)
	writeDoc(t, docsDir, "doc1", validDocContent("doc1", "active", "Hello doc1."))
	appendLog(t, ls, logstore.ActionAdded, []string{"doc1"}, "add doc1")
	if err := s.Sync(context.Background()); err != nil {
		t.Fatalf("first Sync: %v", err)
	}

	c0Before, err := idx.GetChunk(chunkID("doc1", 0))
	if err != nil {
		t.Fatalf("GetChunk doc1: %v", err)
	}

	writeDoc(t, docsDir, "doc2", validDocContent("doc2", "active", "Hello doc2."))
	log2 := appendLog(t, ls, logstore.ActionAdded, []string{"doc2"}, "add doc2")

	if err := s.Sync(context.Background()); err != nil {
		t.Fatalf("second Sync: %v", err)
	}

	if _, err := idx.GetDoc("doc2"); err != nil {
		t.Fatalf("doc2 missing: %v", err)
	}
	c0After, err := idx.GetChunk(chunkID("doc1", 0))
	if err != nil {
		t.Fatal(err)
	}
	if c0After.Content != c0Before.Content {
		t.Errorf("doc1 chunk changed on delta: before=%q after=%q", c0Before.Content, c0After.Content)
	}

	st, err := idx.GetSyncState()
	if err != nil {
		t.Fatal(err)
	}
	if st.LastSyncedLogID != log2 {
		t.Errorf("last_synced=%q want %q", st.LastSyncedLogID, log2)
	}
}

func TestDeletionDetection(t *testing.T) {
	s, docsDir, ls, _, idx := setup(t)
	writeDoc(t, docsDir, "doc1", validDocContent("doc1", "active", "Hello doc1."))
	appendLog(t, ls, logstore.ActionAdded, []string{"doc1"}, "add doc1")
	if err := s.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if err := os.Remove(filepath.Join(docsDir, "doc1.md")); err != nil {
		t.Fatal(err)
	}

	if err := s.Sync(context.Background()); err != nil {
		t.Fatalf("Sync after delete: %v", err)
	}

	if _, err := idx.GetDoc("doc1"); err == nil {
		t.Fatal("expected doc1 removed from index")
	}
	n, _ := idx.ChunkCount("doc1")
	if n != 0 {
		t.Fatalf("expected 0 chunks, got %d", n)
	}
}

func TestChunkUpdateInPlace(t *testing.T) {
	s, docsDir, ls, _, idx := setup(t)
	writeDoc(t, docsDir, "doc1", multiSectionDoc("doc1", []string{"Section A", "Section B"}))
	appendLog(t, ls, logstore.ActionAdded, []string{"doc1"}, "add doc1")
	if err := s.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	nBefore, err := idx.ChunkCount("doc1")
	if err != nil {
		t.Fatal(err)
	}
	if nBefore < 2 {
		t.Fatalf("need >=2 chunks, got %d", nBefore)
	}
	// multiSectionDoc: frontmatter, Summary, Key Decisions, then Section A at index 3.
	idA := chunkID("doc1", 3)
	cABefore, err := idx.GetChunk(idA)
	if err != nil {
		t.Fatal(err)
	}

	writeDoc(t, docsDir, "doc1", multiSectionDoc("doc1", []string{"Section A UPDATED", "Section B UPDATED"}))
	appendLog(t, ls, logstore.ActionChanged, []string{"doc1"}, "edit doc1")
	if err := s.Sync(context.Background()); err != nil {
		t.Fatalf("Sync edit: %v", err)
	}

	nAfter, err := idx.ChunkCount("doc1")
	if err != nil {
		t.Fatal(err)
	}
	if nAfter != nBefore {
		t.Fatalf("chunk count changed: %d -> %d", nBefore, nAfter)
	}
	cAAfter, err := idx.GetChunk(idA)
	if err != nil {
		t.Fatal(err)
	}
	if cAAfter.ChunkID != idA {
		t.Errorf("chunk_id unstable: %s", cAAfter.ChunkID)
	}
	if cAAfter.Content == cABefore.Content {
		t.Error("expected content updated")
	}
	if !strings.Contains(cAAfter.Content, "UPDATED") {
		t.Errorf("content missing update: %q", cAAfter.Content)
	}
}

func TestTrailingChunkCleanup(t *testing.T) {
	s, docsDir, ls, _, idx := setup(t)
	writeDoc(t, docsDir, "doc1", multiSectionDoc("doc1", []string{"A", "B", "C"}))
	appendLog(t, ls, logstore.ActionAdded, []string{"doc1"}, "add")
	if err := s.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	nBefore, _ := idx.ChunkCount("doc1")
	if nBefore < 3 {
		t.Fatalf("want >=3 chunks, got %d", nBefore)
	}

	writeDoc(t, docsDir, "doc1", multiSectionDoc("doc1", []string{"A only"}))
	appendLog(t, ls, logstore.ActionChanged, []string{"doc1"}, "shrink")
	if err := s.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}

	nAfter, _ := idx.ChunkCount("doc1")
	if nAfter >= nBefore {
		t.Fatalf("trailing chunks not cleaned: before=%d after=%d", nBefore, nAfter)
	}
	if _, err := idx.GetChunk(chunkID("doc1", nBefore-1)); err == nil {
		t.Fatal("expected trailing chunk deleted")
	}
}

func TestModelMismatch(t *testing.T) {
	s, docsDir, ls, _, _ := setup(t)
	writeDoc(t, docsDir, "doc1", validDocContent("doc1", "active", "Hello."))
	appendLog(t, ls, logstore.ActionAdded, []string{"doc1"}, "add")
	if err := s.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}

	s.ModelID = "different-model-512"
	err := s.Sync(context.Background())
	if err == nil {
		t.Fatal("expected model mismatch error")
	}
	if !strings.Contains(err.Error(), "--reindex") {
		t.Errorf("error should mention --reindex: %v", err)
	}
}

func TestReindex(t *testing.T) {
	s, docsDir, ls, _, idx := setup(t)
	writeDoc(t, docsDir, "doc1", validDocContent("doc1", "active", "Hello doc1."))
	writeDoc(t, docsDir, "doc2", validDocContent("doc2", "active", "Hello doc2."))
	appendLog(t, ls, logstore.ActionAdded, []string{"doc1"}, "add1")
	appendLog(t, ls, logstore.ActionAdded, []string{"doc2"}, "add2")
	if err := s.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := s.Reindex(context.Background()); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	if _, err := idx.GetDoc("doc1"); err != nil {
		t.Fatalf("doc1 after reindex: %v", err)
	}
	if _, err := idx.GetDoc("doc2"); err != nil {
		t.Fatalf("doc2 after reindex: %v", err)
	}
	st, err := idx.GetSyncState()
	if err != nil {
		t.Fatal(err)
	}
	last, err := ls.LastLogID()
	if err != nil {
		t.Fatal(err)
	}
	if st.LastSyncedLogID != last {
		t.Errorf("sync_state last=%q want %q", st.LastSyncedLogID, last)
	}
	if st.ModelID != testModelID {
		t.Errorf("model_id=%q", st.ModelID)
	}
}

func TestDeprecatedDocStillIndexedButExcludedFromSearch(t *testing.T) {
	s, docsDir, ls, _, idx := setup(t)
	writeDoc(t, docsDir, "dep", validDocContent("dep", "deprecated", "Deprecated content unique-token-xyz."))
	appendLog(t, ls, logstore.ActionDeprecated, []string{"dep"}, "deprecate")
	if err := s.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}

	doc, err := idx.GetDoc("dep")
	if err != nil {
		t.Fatalf("deprecated doc should be in index: %v", err)
	}
	if doc.Status != "deprecated" {
		t.Errorf("status=%q", doc.Status)
	}
	n, _ := idx.ChunkCount("dep")
	if n == 0 {
		t.Fatal("deprecated doc should still have chunks")
	}

	emb, err := s.Embedder.Embed("unique-token-xyz")
	if err != nil {
		t.Fatal(err)
	}
	results, err := idx.Search(emb, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.DocID == "dep" {
			t.Fatal("Search returned deprecated doc")
		}
	}
}

func TestMalformedLogLine(t *testing.T) {
	s, docsDir, ls, _, _ := setup(t)
	writeDoc(t, docsDir, "doc1", validDocContent("doc1", "active", "Hello."))
	appendLog(t, ls, logstore.ActionAdded, []string{"doc1"}, "add")
	if err := s.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}

	root := filepath.Dir(docsDir)
	f, err := os.OpenFile(filepath.Join(root, "memory.jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{not valid json\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	err = s.Sync(context.Background())
	if err == nil {
		t.Fatal("expected error on malformed log")
	}
}
