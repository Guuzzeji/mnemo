package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validYAML = `
system:
  log_file: ".mnemo/memory_log.jsonl"
  docs_dir: ".mnemo/docs"
database:
  path: ".mnemo/index.db"
embeddings:
  model_repo: "onnx-community/embeddinggemma-300m-ONNX"
  model_path: ".mnemo/models/embeddinggemma-300m.onnx"
  dimensions: 768
  search_top_k: 5
  chunking:
    split_on: ["## "]
    max_chunk_tokens: 500
    overlap_tokens: 50
taxonomy:
  allowed_categories:
    - backend
    - frontend
    - devops
    - architecture
  key_terms:
    "authentication": ["auth", "jwt", "session", "login"]
    "database": ["db", "sql", "sqlite", "schema"]
`

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadValidConfig(t *testing.T) {
	path := writeTempConfig(t, validYAML)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.System.LogFile != ".mnemo/memory_log.jsonl" {
		t.Errorf("LogFile = %q, want %q", cfg.System.LogFile, ".mnemo/memory_log.jsonl")
	}
	if cfg.System.DocsDir != ".mnemo/docs" {
		t.Errorf("DocsDir = %q, want %q", cfg.System.DocsDir, ".mnemo/docs")
	}
	if cfg.Database.Path != ".mnemo/index.db" {
		t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, ".mnemo/index.db")
	}
	if cfg.Embeddings.ModelRepo != "onnx-community/embeddinggemma-300m-ONNX" {
		t.Errorf("ModelRepo = %q", cfg.Embeddings.ModelRepo)
	}
	if cfg.Embeddings.ModelPath != ".mnemo/models/embeddinggemma-300m.onnx" {
		t.Errorf("ModelPath = %q", cfg.Embeddings.ModelPath)
	}
	if cfg.Embeddings.Dimensions != 768 {
		t.Errorf("Dimensions = %d, want 768", cfg.Embeddings.Dimensions)
	}
	if cfg.Embeddings.SearchTopK != 5 {
		t.Errorf("SearchTopK = %d, want 5", cfg.Embeddings.SearchTopK)
	}
	if len(cfg.Embeddings.Chunking.SplitOn) != 1 || cfg.Embeddings.Chunking.SplitOn[0] != "## " {
		t.Errorf("SplitOn = %v, want [## ]", cfg.Embeddings.Chunking.SplitOn)
	}
	if cfg.Embeddings.Chunking.MaxChunkTokens != 500 {
		t.Errorf("MaxChunkTokens = %d, want 500", cfg.Embeddings.Chunking.MaxChunkTokens)
	}
	if cfg.Embeddings.Chunking.OverlapTokens != 50 {
		t.Errorf("OverlapTokens = %d, want 50", cfg.Embeddings.Chunking.OverlapTokens)
	}
	if len(cfg.Taxonomy.AllowedCategories) != 4 {
		t.Errorf("AllowedCategories len = %d, want 4", len(cfg.Taxonomy.AllowedCategories))
	}
	if len(cfg.Taxonomy.KeyTerms) != 2 {
		t.Errorf("KeyTerms len = %d, want 2", len(cfg.Taxonomy.KeyTerms))
	}
	authTerms, ok := cfg.Taxonomy.KeyTerms["authentication"]
	if !ok || len(authTerms) != 4 {
		t.Errorf("KeyTerms[authentication] = %v, want 4 terms", authTerms)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("Load() on missing file should return error")
	}
	if !strings.Contains(err.Error(), "--init") {
		t.Errorf("error should mention --init, got: %v", err)
	}
}

func TestValidateAllErrors(t *testing.T) {
	cfg := &Config{
		System:   SystemConfig{LogFile: "a", DocsDir: "b"},
		Database: DatabaseConfig{Path: "c"},
		Embeddings: EmbeddingsConfig{
			ModelRepo:  "repo",
			ModelPath:  "path",
			Dimensions: 0,
			SearchTopK: 0,
			Chunking: ChunkingConfig{
				MaxChunkTokens: 0,
			},
		},
		Taxonomy: TaxonomyConfig{},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() should return errors for invalid config")
	}
	errMsg := err.Error()

	mustContain := []string{
		"dimensions",
		"search_top_k",
		"split_on",
		"max_chunk_tokens",
		"allowed_categories",
	}
	for _, want := range mustContain {
		if !strings.Contains(errMsg, want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
}

func TestValidateValidConfig(t *testing.T) {
	path := writeTempConfig(t, validYAML)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() on valid config: %v", err)
	}
}

func TestResolvePath(t *testing.T) {
	path := writeTempConfig(t, validYAML)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	dir := filepath.Dir(path)

	got := cfg.ResolvePath(".mnemo/memory_log.jsonl")
	want := filepath.Join(dir, ".mnemo/memory_log.jsonl")
	if got != want {
		t.Errorf("ResolvePath() = %q, want %q", got, want)
	}

	got = cfg.ResolvePath("/absolute/path")
	want = "/absolute/path"
	if got != want {
		t.Errorf("ResolvePath(absolute) = %q, want %q", got, want)
	}
}
