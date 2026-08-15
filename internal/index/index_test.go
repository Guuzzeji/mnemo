package index

import (
	"testing"
)

func testIndex(t *testing.T) *Index {
	t.Helper()
	idx, err := Open(t.TempDir()+"/test.db", 4)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx
}

func TestOpenCreatesTables(t *testing.T) {
	idx := testIndex(t)

	tables := []string{"docs", "chunks", "sync_state"}
	for _, tbl := range tables {
		var name string
		err := idx.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", tbl, err)
		}
	}
}

func TestUpsertDocRoundtrip(t *testing.T) {
	idx := testIndex(t)

	doc := DocRecord{
		ID:        "doc-1",
		Path:      "/docs/readme.md",
		TagsJSON:  `["reference","api"]`,
		Status:    "active",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}
	if err := idx.UpsertDoc(doc); err != nil {
		t.Fatalf("UpsertDoc: %v", err)
	}

	got, err := idx.GetDoc("doc-1")
	if err != nil {
		t.Fatalf("GetDoc: %v", err)
	}
	if got.ID != doc.ID || got.Path != doc.Path || got.TagsJSON != doc.TagsJSON || got.Status != doc.Status {
		t.Errorf("got %+v, want %+v", got, doc)
	}
}

func TestUpsertDocOverwrite(t *testing.T) {
	idx := testIndex(t)

	doc := DocRecord{ID: "doc-1", Path: "/a.md", TagsJSON: `["x"]`, Status: "active", UpdatedAt: "2026-01-01T00:00:00Z"}
	if err := idx.UpsertDoc(doc); err != nil {
		t.Fatal(err)
	}

	doc.Path = "/b.md"
	doc.Status = "deprecated"
	if err := idx.UpsertDoc(doc); err != nil {
		t.Fatal(err)
	}

	got, err := idx.GetDoc("doc-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/b.md" || got.Status != "deprecated" {
		t.Errorf("overwrite failed: got %+v", got)
	}
}

func TestUpsertChunkAndSearch(t *testing.T) {
	idx := testIndex(t)

	idx.UpsertDoc(DocRecord{ID: "doc-a", Path: "/a.md", TagsJSON: `["ref"]`, Status: "active", UpdatedAt: "2026-01-01T00:00:00Z"})

	chunks := []ChunkRecord{
		{ChunkID: "c1", DocID: "doc-a", ChunkIndex: 0, Heading: "Intro", Content: "hello world", Embedding: []float32{1, 0, 0, 0}},
		{ChunkID: "c2", DocID: "doc-a", ChunkIndex: 1, Heading: "Middle", Content: "foo bar", Embedding: []float32{0, 1, 0, 0}},
		{ChunkID: "c3", DocID: "doc-a", ChunkIndex: 2, Heading: "End", Content: "baz qux", Embedding: []float32{0, 0, 1, 0}},
	}
	for _, c := range chunks {
		if err := idx.UpsertChunk(c); err != nil {
			t.Fatalf("UpsertChunk %s: %v", c.ChunkID, err)
		}
	}

	results, err := idx.Search([]float32{1, 0, 0, 0}, nil, 3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].ChunkID != "c1" {
		t.Errorf("expected nearest=c1, got %s (dist=%f)", results[0].ChunkID, results[0].Distance)
	}
	if results[0].Distance > 0.001 {
		t.Errorf("self-distance should be ~0, got %f", results[0].Distance)
	}
}

func TestSearchFiltersTags(t *testing.T) {
	idx := testIndex(t)

	idx.UpsertDoc(DocRecord{ID: "d1", Path: "/a", TagsJSON: `["api"]`, Status: "active", UpdatedAt: "2026-01-01T00:00:00Z"})
	idx.UpsertDoc(DocRecord{ID: "d2", Path: "/b", TagsJSON: `["guide"]`, Status: "active", UpdatedAt: "2026-01-01T00:00:00Z"})

	idx.UpsertChunk(ChunkRecord{ChunkID: "c1", DocID: "d1", ChunkIndex: 0, Heading: "h", Content: "c1", Embedding: []float32{1, 0, 0, 0}})
	idx.UpsertChunk(ChunkRecord{ChunkID: "c2", DocID: "d2", ChunkIndex: 0, Heading: "h", Content: "c2", Embedding: []float32{1, 0, 0, 0}})

	results, err := idx.Search([]float32{1, 0, 0, 0}, []string{"api"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].DocID != "d1" {
		t.Errorf("expected 1 result from d1, got %+v", results)
	}
}

func TestSearchExcludesDeprecated(t *testing.T) {
	idx := testIndex(t)

	idx.UpsertDoc(DocRecord{ID: "d1", Path: "/a", TagsJSON: `["x"]`, Status: "active", UpdatedAt: "2026-01-01T00:00:00Z"})
	idx.UpsertDoc(DocRecord{ID: "d2", Path: "/b", TagsJSON: `["x"]`, Status: "deprecated", UpdatedAt: "2026-01-01T00:00:00Z"})

	idx.UpsertChunk(ChunkRecord{ChunkID: "c1", DocID: "d1", ChunkIndex: 0, Heading: "h", Content: "active", Embedding: []float32{1, 0, 0, 0}})
	idx.UpsertChunk(ChunkRecord{ChunkID: "c2", DocID: "d2", ChunkIndex: 0, Heading: "h", Content: "deprecated", Embedding: []float32{1, 0, 0, 0}})

	results, err := idx.Search([]float32{1, 0, 0, 0}, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].DocID != "d1" {
		t.Errorf("expected only active doc, got %+v", results)
	}
}

func TestDeleteDoc(t *testing.T) {
	idx := testIndex(t)

	idx.UpsertDoc(DocRecord{ID: "d1", Path: "/a", TagsJSON: `["x"]`, Status: "active", UpdatedAt: "2026-01-01T00:00:00Z"})
	idx.UpsertChunk(ChunkRecord{ChunkID: "c1", DocID: "d1", ChunkIndex: 0, Heading: "h", Content: "text", Embedding: []float32{1, 0, 0, 0}})

	if err := idx.DeleteDoc("d1"); err != nil {
		t.Fatal(err)
	}

	_, err := idx.GetDoc("d1")
	if err == nil {
		t.Error("expected error for deleted doc")
	}

	results, err := idx.Search([]float32{1, 0, 0, 0}, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results after delete, got %d", len(results))
	}
}

func TestDeleteChunksAfter(t *testing.T) {
	idx := testIndex(t)

	idx.UpsertDoc(DocRecord{ID: "d1", Path: "/a", TagsJSON: `["x"]`, Status: "active", UpdatedAt: "2026-01-01T00:00:00Z"})
	for i := 0; i < 5; i++ {
		idx.UpsertChunk(ChunkRecord{
			ChunkID:    "c" + string(rune('0'+i)),
			DocID:      "d1",
			ChunkIndex: i,
			Heading:    "h",
			Content:    "text",
			Embedding:  []float32{float32(i), 0, 0, 0},
		})
	}

	if err := idx.DeleteChunksAfter("d1", 3); err != nil {
		t.Fatal(err)
	}

	var count int
	idx.db.QueryRow("SELECT count(*) FROM chunks WHERE doc_id='d1'").Scan(&count)
	if count != 3 {
		t.Errorf("expected 3 chunks, got %d", count)
	}
}

func TestSyncStateRoundtrip(t *testing.T) {
	idx := testIndex(t)

	state := SyncState{LastSyncedLogID: "uuid-abc", ModelID: "model-v1", SyncedAt: "2026-01-01T00:00:00Z"}
	if err := idx.SetSyncState(state); err != nil {
		t.Fatal(err)
	}

	got, err := idx.GetSyncState()
	if err != nil {
		t.Fatal(err)
	}
	if got != state {
		t.Errorf("got %+v, want %+v", got, state)
	}

	state.ModelID = "model-v2"
	idx.SetSyncState(state)
	got, _ = idx.GetSyncState()
	if got.ModelID != "model-v2" {
		t.Errorf("overwrite failed: got %s", got.ModelID)
	}
}

func TestWipe(t *testing.T) {
	idx := testIndex(t)

	idx.UpsertDoc(DocRecord{ID: "d1", Path: "/a", TagsJSON: `["x"]`, Status: "active", UpdatedAt: "t"})
	idx.UpsertChunk(ChunkRecord{ChunkID: "c1", DocID: "d1", ChunkIndex: 0, Heading: "h", Content: "text", Embedding: []float32{1, 0, 0, 0}})
	idx.SetSyncState(SyncState{LastSyncedLogID: "x", ModelID: "m", SyncedAt: "t"})

	if err := idx.Wipe(); err != nil {
		t.Fatal(err)
	}

	var count int
	idx.db.QueryRow("SELECT count(*) FROM docs").Scan(&count)
	if count != 0 {
		t.Errorf("docs not empty after wipe: %d", count)
	}
	idx.db.QueryRow("SELECT count(*) FROM chunks").Scan(&count)
	if count != 0 {
		t.Errorf("chunks not empty after wipe: %d", count)
	}
}

func TestSearchEmptyTagsNoFilter(t *testing.T) {
	idx := testIndex(t)

	idx.UpsertDoc(DocRecord{ID: "d1", Path: "/a", TagsJSON: `["api","guide"]`, Status: "active", UpdatedAt: "t"})
	idx.UpsertChunk(ChunkRecord{ChunkID: "c1", DocID: "d1", ChunkIndex: 0, Heading: "h", Content: "text", Embedding: []float32{1, 0, 0, 0}})

	results, err := idx.Search([]float32{1, 0, 0, 0}, []string{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result with empty tags filter, got %d", len(results))
	}
}

func TestSerializeDeserialize(t *testing.T) {
	v := []float32{1.5, -2.0, 0, 3.14}
	b := serializeFloat32(v)
	got := deserializeFloat32(b)
	if len(got) != len(v) {
		t.Fatalf("len mismatch: %d vs %d", len(got), len(v))
	}
	for i := range v {
		if v[i] != got[i] {
			t.Errorf("index %d: got %f, want %f", i, got[i], v[i])
		}
	}
}
