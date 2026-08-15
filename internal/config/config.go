package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	System     SystemConfig     `yaml:"system"`
	Database   DatabaseConfig   `yaml:"database"`
	Embeddings EmbeddingsConfig `yaml:"embeddings"`
	Taxonomy   TaxonomyConfig   `yaml:"taxonomy"`
	configDir  string
}

type SystemConfig struct {
	LogFile string `yaml:"log_file"`
	DocsDir string `yaml:"docs_dir"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type EmbeddingsConfig struct {
	ModelRepo   string         `yaml:"model_repo"`
	ModelPath   string         `yaml:"model_path"`
	ModelSHA256 string         `yaml:"model_sha256,omitempty"`
	Dimensions  int            `yaml:"dimensions"`
	SearchTopK  int            `yaml:"search_top_k"`
	Chunking    ChunkingConfig `yaml:"chunking"`
}

type ChunkingConfig struct {
	SplitOn        []string `yaml:"split_on"`
	MaxChunkTokens int      `yaml:"max_chunk_tokens"`
	OverlapTokens  int      `yaml:"overlap_tokens"`
}

type TaxonomyConfig struct {
	AllowedCategories []string            `yaml:"allowed_categories"`
	KeyTerms          map[string][]string `yaml:"key_terms"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config file not found at %s; run with --init to scaffold", path)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid YAML in %s: %w", path, err)
	}
	cfg.configDir = filepath.Dir(path)
	return &cfg, nil
}

func (c *Config) ResolvePath(rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(c.configDir, rel)
}

func (c *Config) Validate() error {
	var errs []error

	if len(c.Taxonomy.AllowedCategories) == 0 {
		errs = append(errs, errors.New("allowed_categories must not be empty"))
	}
	if len(c.Embeddings.Chunking.SplitOn) == 0 {
		errs = append(errs, errors.New("split_on must not be empty"))
	}
	if c.Embeddings.Chunking.MaxChunkTokens <= 0 {
		errs = append(errs, fmt.Errorf("max_chunk_tokens must be > 0, got %d", c.Embeddings.Chunking.MaxChunkTokens))
	}
	if c.Embeddings.Dimensions <= 0 {
		errs = append(errs, fmt.Errorf("dimensions must be > 0, got %d", c.Embeddings.Dimensions))
	}
	if c.Embeddings.SearchTopK <= 0 {
		errs = append(errs, fmt.Errorf("search_top_k must be > 0, got %d", c.Embeddings.SearchTopK))
	}

	return errors.Join(errs...)
}
