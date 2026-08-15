package initcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultConfig = `system:
  log_file: ".memory-mcp/memory_log.jsonl"
  docs_dir: ".memory-mcp/docs"
database:
  path: ".memory-mcp/index.db"
embeddings:
  model_repo: "onnx-community/embeddinggemma-300m-ONNX"
  model_path: ".memory-mcp/models/embeddinggemma-300m.onnx"
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

var gitignoreEntries = []string{
	".memory-mcp/index.db",
	".memory-mcp/index.db-shm",
	".memory-mcp/index.db-wal",
	".memory-mcp/models/",
	".memory-mcp/mnemo",
	".memory-mcp/server.log",
	".sisyphus/",
}

// Run scaffolds a fresh repo with memory server defaults.
func Run(dir string) error {
	cfgPath := filepath.Join(dir, ".memory_config.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		return fmt.Errorf("already initialized: %s exists", cfgPath)
	}

	if err := os.WriteFile(cfgPath, []byte(defaultConfig), 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	for _, d := range []string{".memory-mcp", ".memory-mcp/docs"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0755); err != nil {
			return fmt.Errorf("creating %s: %w", d, err)
		}
	}

	gitignorePath := filepath.Join(dir, ".gitignore")
	existing, _ := os.ReadFile(gitignorePath)
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(existing), "\n") {
		seen[strings.TrimSpace(line)] = true
	}

	var toAdd []string
	for _, entry := range gitignoreEntries {
		if !seen[entry] {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) > 0 {
		f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("opening .gitignore: %w", err)
		}
		defer f.Close()

		for _, line := range toAdd {
			if _, err := fmt.Fprintf(f, "%s\n", line); err != nil {
				return fmt.Errorf("writing .gitignore: %w", err)
			}
		}
	}

	return nil
}
