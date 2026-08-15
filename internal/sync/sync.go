package sync

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/Guuzzeji/ai-shared-memory/internal/chunking"
	"github.com/Guuzzeji/ai-shared-memory/internal/docs"
	"github.com/Guuzzeji/ai-shared-memory/internal/embeddings"
	"github.com/Guuzzeji/ai-shared-memory/internal/index"
	"github.com/Guuzzeji/ai-shared-memory/internal/logstore"
)

// Syncer reconciles the log + docs into the index.
type Syncer struct {
	Log      *logstore.Store
	Docs     *docs.Store
	Chunker  *chunking.Chunker
	Embedder embeddings.Embedder
	Index    *index.Index
	ModelID  string
}

// Sync performs the delta sync. Returns error on failure.
func (s *Syncer) Sync(ctx context.Context) error {
	return s.sync(ctx, false)
}

// Reindex wipes the index and replays the entire log.
func (s *Syncer) Reindex(ctx context.Context) error {
	if err := s.Index.Wipe(); err != nil {
		return fmt.Errorf("wipe index: %w", err)
	}
	return s.sync(ctx, true)
}

func (s *Syncer) sync(ctx context.Context, full bool) error {
	_ = ctx

	var lastSynced string
	if !full {
		st, err := s.Index.GetSyncState()
		if err == nil {
			if st.ModelID != "" && st.ModelID != s.ModelID {
				return errors.New("index built with different model; run --reindex")
			}
			lastSynced = st.LastSyncedLogID
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("get sync state: %w", err)
		}
	}

	entries, err := s.Log.ReadAfter(lastSynced)
	if err != nil {
		return fmt.Errorf("read log: %w", err)
	}

	affected := make(map[string]struct{})
	var lastLogID string
	for _, e := range entries {
		for _, id := range e.TargetDocs {
			affected[id] = struct{}{}
		}
		lastLogID = e.LogID
	}

	indexed, err := s.Index.ListDocs()
	if err != nil {
		return fmt.Errorf("list indexed docs: %w", err)
	}
	for _, d := range indexed {
		if !s.Docs.Exists(d.ID) {
			if err := s.Index.DeleteDoc(d.ID); err != nil {
				return fmt.Errorf("delete missing doc %s: %w", d.ID, err)
			}
			delete(affected, d.ID)
		}
	}

	for id := range affected {
		if err := s.processDoc(id); err != nil {
			return err
		}
	}

	if full {
		if lid, err := s.Log.LastLogID(); err == nil {
			lastLogID = lid
		}
	} else if lastLogID == "" {
		if lastSynced != "" {
			return nil
		}
	}

	return s.Index.SetSyncState(index.SyncState{
		LastSyncedLogID: lastLogID,
		ModelID:         s.ModelID,
		SyncedAt:        time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Syncer) processDoc(id string) error {
	content, err := s.Docs.Get(id)
	if err != nil {
		return fmt.Errorf("read doc %s: %w", id, err)
	}

	doc, err := docs.Parse([]byte(content))
	if err != nil {
		return fmt.Errorf("parse doc %s: %w", id, err)
	}

	chunks, err := s.Chunker.Chunk(id, content)
	if err != nil {
		return fmt.Errorf("chunk doc %s: %w", id, err)
	}

	path := id + ".md"
	updatedAt := time.Now().UTC().Format(time.RFC3339)
	if listed, err := s.Docs.List(); err == nil {
		for _, d := range listed {
			if d.ID == id && d.Path != "" {
				path = d.Path
				if fi, err := os.Stat(d.Path); err == nil {
					updatedAt = fi.ModTime().UTC().Format(time.RFC3339)
				}
				break
			}
		}
	}

	tagsJSON, err := json.Marshal(doc.Tags)
	if err != nil {
		return fmt.Errorf("marshal tags: %w", err)
	}

	// vec0 has no true UPSERT; clear then insert keeps chunk_id stable.
	if err := s.Index.DeleteChunksAfter(id, 0); err != nil {
		return fmt.Errorf("clear chunks %s: %w", id, err)
	}

	for _, ch := range chunks {
		emb, err := s.Embedder.Embed(ch.Content)
		if err != nil {
			return fmt.Errorf("embed chunk %s[%d]: %w", id, ch.ChunkIndex, err)
		}
		cid := stableChunkID(id, ch.ChunkIndex)
		if err := s.Index.UpsertChunk(index.ChunkRecord{
			ChunkID:    cid,
			DocID:      id,
			ChunkIndex: ch.ChunkIndex,
			Heading:    ch.Heading,
			Content:    ch.Content,
			Embedding:  emb,
		}); err != nil {
			return fmt.Errorf("upsert chunk %s: %w", cid, err)
		}
	}

	if err := s.Index.UpsertDoc(index.DocRecord{
		ID:        id,
		Path:      path,
		TagsJSON:  string(tagsJSON),
		Status:    doc.Status,
		UpdatedAt: updatedAt,
	}); err != nil {
		return fmt.Errorf("upsert doc %s: %w", id, err)
	}

	return nil
}

func stableChunkID(docID string, chunkIndex int) string {
	sum := sha256.Sum256([]byte(docID + strconv.Itoa(chunkIndex)))
	return hex.EncodeToString(sum[:])
}
