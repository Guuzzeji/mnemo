package index

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	_ "modernc.org/sqlite"
	_ "modernc.org/sqlite/vec"
)

type DocRecord struct {
	ID, Path, TagsJSON, Status, UpdatedAt string
}

type ChunkRecord struct {
	ChunkID, DocID   string
	ChunkIndex       int
	Heading, Content string
	Embedding        []float32
}

type SyncState struct {
	LastSyncedLogID, ModelID, SyncedAt string
}

type SearchResult struct {
	ChunkID, DocID, Heading, Content string
	Distance                         float64
}

type Index struct {
	db         *sql.DB
	dimensions int
}

func Open(path string, dimensions int) (*Index, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set busy_timeout: %w", err)
	}

	ddl := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS docs (
			id TEXT PRIMARY KEY,
			path TEXT NOT NULL,
			tags TEXT NOT NULL,
			status TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE VIRTUAL TABLE IF NOT EXISTS chunks USING vec0(
			chunk_id TEXT PRIMARY KEY,
			doc_id TEXT,
			chunk_index INTEGER,
			heading TEXT,
			content TEXT,
			embedding float[%d]
		);

		CREATE TABLE IF NOT EXISTS sync_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			last_synced_log_id TEXT NOT NULL,
			model_id TEXT NOT NULL,
			synced_at TEXT NOT NULL
		);
	`, dimensions)

	if _, err := db.Exec(ddl); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &Index{db: db, dimensions: dimensions}, nil
}

func (idx *Index) UpsertDoc(doc DocRecord) error {
	_, err := idx.db.Exec(
		"INSERT OR REPLACE INTO docs (id, path, tags, status, updated_at) VALUES (?, ?, ?, ?, ?)",
		doc.ID, doc.Path, doc.TagsJSON, doc.Status, doc.UpdatedAt,
	)
	return err
}

func (idx *Index) GetDoc(id string) (DocRecord, error) {
	var doc DocRecord
	err := idx.db.QueryRow(
		"SELECT id, path, tags, status, updated_at FROM docs WHERE id=?", id,
	).Scan(&doc.ID, &doc.Path, &doc.TagsJSON, &doc.Status, &doc.UpdatedAt)
	return doc, err
}

func (idx *Index) ListDocs() ([]DocRecord, error) {
	rows, err := idx.db.Query("SELECT id, path, tags, status, updated_at FROM docs")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []DocRecord
	for rows.Next() {
		var doc DocRecord
		if err := rows.Scan(&doc.ID, &doc.Path, &doc.TagsJSON, &doc.Status, &doc.UpdatedAt); err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}

func (idx *Index) GetChunk(chunkID string) (ChunkRecord, error) {
	var c ChunkRecord
	err := idx.db.QueryRow(
		`SELECT chunk_id, doc_id, chunk_index, heading, content FROM chunks WHERE chunk_id=?`,
		chunkID,
	).Scan(&c.ChunkID, &c.DocID, &c.ChunkIndex, &c.Heading, &c.Content)
	return c, err
}

func (idx *Index) ChunkCount(docID string) (int, error) {
	var n int
	err := idx.db.QueryRow("SELECT COUNT(*) FROM chunks WHERE doc_id=?", docID).Scan(&n)
	return n, err
}

func (idx *Index) DeleteDoc(id string) error {
	tx, err := idx.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM chunks WHERE doc_id=?", id); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM docs WHERE id=?", id); err != nil {
		return err
	}
	return tx.Commit()
}

func (idx *Index) UpsertChunk(chunk ChunkRecord) error {
	embeddingBlob := serializeFloat32(chunk.Embedding)
	_, err := idx.db.Exec(
		`INSERT OR REPLACE INTO chunks (chunk_id, doc_id, chunk_index, heading, content, embedding)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		chunk.ChunkID, chunk.DocID, chunk.ChunkIndex, chunk.Heading, chunk.Content, embeddingBlob,
	)
	return err
}

func (idx *Index) DeleteChunksAfter(docID string, fromIndex int) error {
	_, err := idx.db.Exec("DELETE FROM chunks WHERE doc_id=? AND chunk_index>=?", docID, fromIndex)
	return err
}

func (idx *Index) GetSyncState() (SyncState, error) {
	var s SyncState
	err := idx.db.QueryRow(
		"SELECT last_synced_log_id, model_id, synced_at FROM sync_state WHERE id=1",
	).Scan(&s.LastSyncedLogID, &s.ModelID, &s.SyncedAt)
	return s, err
}

func (idx *Index) SetSyncState(state SyncState) error {
	_, err := idx.db.Exec(
		`INSERT OR REPLACE INTO sync_state (id, last_synced_log_id, model_id, synced_at)
		 VALUES (1, ?, ?, ?)`,
		state.LastSyncedLogID, state.ModelID, state.SyncedAt,
	)
	return err
}

func (idx *Index) Search(query []float32, tags []string, topK int) ([]SearchResult, error) {
	queryBlob := serializeFloat32(query)

	var sb strings.Builder
	sb.WriteString(`
		SELECT chunks.chunk_id, chunks.doc_id, chunks.heading, chunks.content,
		       vec_distance_cosine(chunks.embedding, ?) AS distance
		FROM chunks
		JOIN docs ON chunks.doc_id = docs.id
		WHERE docs.status = 'active'
	`)

	args := []any{queryBlob}

	if len(tags) > 0 {
		placeholders := make([]string, len(tags))
		for i, tag := range tags {
			placeholders[i] = "?"
			args = append(args, tag)
		}
		sb.WriteString(fmt.Sprintf(
			" AND EXISTS (SELECT 1 FROM json_each(docs.tags) WHERE json_each.value IN (%s))",
			strings.Join(placeholders, ","),
		))
	}

	sb.WriteString(" ORDER BY distance LIMIT ?")
	args = append(args, topK)

	rows, err := idx.db.Query(sb.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.ChunkID, &r.DocID, &r.Heading, &r.Content, &r.Distance); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (idx *Index) Wipe() error {
	_, err := idx.db.Exec(`
		DROP TABLE IF EXISTS chunks;
		DROP TABLE IF EXISTS docs;
		DROP TABLE IF EXISTS sync_state;
	`)
	if err != nil {
		return err
	}

	ddl := fmt.Sprintf(`
		CREATE TABLE docs (
			id TEXT PRIMARY KEY,
			path TEXT NOT NULL,
			tags TEXT NOT NULL,
			status TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE VIRTUAL TABLE chunks USING vec0(
			chunk_id TEXT PRIMARY KEY,
			doc_id TEXT,
			chunk_index INTEGER,
			heading TEXT,
			content TEXT,
			embedding float[%d]
		);

		CREATE TABLE sync_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			last_synced_log_id TEXT NOT NULL,
			model_id TEXT NOT NULL,
			synced_at TEXT NOT NULL
		);
	`, idx.dimensions)
	_, err = idx.db.Exec(ddl)
	return err
}

func (idx *Index) Close() error {
	return idx.db.Close()
}

func serializeFloat32(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

func deserializeFloat32(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}
