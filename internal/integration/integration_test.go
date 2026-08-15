package integration

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Guuzzeji/mnemo/internal/chunking"
	"github.com/Guuzzeji/mnemo/internal/config"
	"github.com/Guuzzeji/mnemo/internal/docs"
	"github.com/Guuzzeji/mnemo/internal/index"
	"github.com/Guuzzeji/mnemo/internal/logstore"
	"github.com/Guuzzeji/mnemo/internal/mcp"
	"github.com/Guuzzeji/mnemo/internal/sync"
	"github.com/Guuzzeji/mnemo/internal/taxonomy"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type constEmbedder struct{ dims int }

func (c *constEmbedder) Embed(string) ([]float32, error) {
	v := make([]float32, c.dims)
	for i := range v {
		v[i] = 1 / float32(math.Sqrt(float64(c.dims)))
	}
	return v, nil
}

func (c *constEmbedder) CountTokens(text string) int { return len(text) / 4 }

type goldenResult struct {
	DocID   string `json:"doc_id"`
	ChunkID string `json:"chunk_id"`
	Heading string `json:"heading"`
}

type goldenFile struct {
	Results []goldenResult `json:"results"`
}

func TestGoldenSearch(t *testing.T) {
	tmp := t.TempDir()
	if err := copyDir("testdata/memory", tmp); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(filepath.Join(tmp, ".memory_config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	log, err := logstore.New(filepath.Join(tmp, "memory_log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	docsStore, err := docs.New(filepath.Join(tmp, "docs"), cfg.Taxonomy.AllowedCategories)
	if err != nil {
		t.Fatal(err)
	}
	embed := &constEmbedder{dims: cfg.Embeddings.Dimensions}
	chunker := chunking.New(cfg.Embeddings.Chunking.SplitOn, cfg.Embeddings.Chunking.MaxChunkTokens, cfg.Embeddings.Chunking.OverlapTokens, embed)
	idx, err := index.Open(filepath.Join(tmp, "index.db"), cfg.Embeddings.Dimensions)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()

	syncer := &sync.Syncer{
		Log:      log,
		Docs:     docsStore,
		Chunker:  chunker,
		Embedder: embed,
		Index:    idx,
		ModelID:  "golden-768",
	}
	if err := syncer.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}

	srv, err := mcp.NewServer(mcp.Deps{
		Config:   cfg,
		Log:      log,
		Docs:     docsStore,
		Index:    idx,
		Syncer:   syncer,
		Taxonomy: taxonomy.New(cfg.Taxonomy.AllowedCategories, cfg.Taxonomy.KeyTerms),
		Embedder: embed,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	t1, t2 := sdk.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, t1, nil); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	cs, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	res, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "semantic_search",
		Arguments: map[string]any{"query": "authentication"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool call failed: %v", res.Content)
	}

	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Results []goldenResult `json:"results"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	sort.Slice(out.Results, func(i, j int) bool { return out.Results[i].ChunkID < out.Results[j].ChunkID })

	got, err := json.MarshalIndent(goldenFile{Results: out.Results}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/golden/search.json")
	if err != nil {
		t.Fatalf("missing golden file; actual results:\n%s", got)
	}
	if strings.TrimSpace(string(got)) != strings.TrimSpace(string(want)) {
		t.Fatalf("golden mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := os.MkdirAll(dstPath, 0o755); err != nil {
				return err
			}
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dstPath, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
