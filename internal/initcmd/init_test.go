package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Guuzzeji/ai-shared-memory/internal/config"
)

func TestRun_CreatesConfigFile(t *testing.T) {
	dir := t.TempDir()
	if err := Run(dir); err != nil {
		t.Fatalf("Run() failed: %v", err)
	}

	cfgPath := filepath.Join(dir, ".memory_config.yaml")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Fatal("config file not created")
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load() failed: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config.Validate() failed: %v", err)
	}
}

func TestRun_CreatesMemoryDirs(t *testing.T) {
	dir := t.TempDir()
	if err := Run(dir); err != nil {
		t.Fatalf("Run() failed: %v", err)
	}

	for _, d := range []string{".memory-mcp", ".memory-mcp/docs"} {
		info, err := os.Stat(filepath.Join(dir, d))
		if os.IsNotExist(err) {
			t.Errorf("directory %s not created", d)
		} else if !info.IsDir() {
			t.Errorf("%s is not a directory", d)
		}
	}
}

func TestRun_AppendsGitignore(t *testing.T) {
	dir := t.TempDir()
	if err := Run(dir); err != nil {
		t.Fatalf("Run() failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, ".memory-mcp/index.db") {
		t.Error(".gitignore missing .memory-mcp/index.db")
	}
	if !strings.Contains(content, ".memory-mcp/index.db-shm") {
		t.Error(".gitignore missing .memory-mcp/index.db-shm")
	}
	if !strings.Contains(content, ".memory-mcp/index.db-wal") {
		t.Error(".gitignore missing .memory-mcp/index.db-wal")
	}
	if !strings.Contains(content, ".memory-mcp/models/") {
		t.Error(".gitignore missing .memory-mcp/models/")
	}
	if !strings.Contains(content, ".memory-mcp/mnemo") {
		t.Error(".gitignore missing .memory-mcp/mnemo")
	}
	if !strings.Contains(content, ".memory-mcp/server.log") {
		t.Error(".gitignore missing .memory-mcp/server.log")
	}
	if !strings.Contains(content, ".sisyphus/") {
		t.Error(".gitignore missing .sisyphus/")
	}
}

func TestRun_Idempotent_NoDuplicateGitignore(t *testing.T) {
	dir := t.TempDir()
	if err := Run(dir); err != nil {
		t.Fatalf("first Run() failed: %v", err)
	}
	_ = Run(dir) // second run errors on config, but gitignore must stay clean

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}

	content := string(data)

	// Exact line matches — substring counting is wrong because
	// ".memory-mcp/index.db" is a prefix of the -shm/-wal entries.
	lines := strings.Split(content, "\n")
	for _, want := range []string{
		".memory-mcp/index.db",
		".memory-mcp/index.db-shm",
		".memory-mcp/index.db-wal",
		".memory-mcp/models/",
		".memory-mcp/mnemo",
		".memory-mcp/server.log",
		".sisyphus/",
	} {
		n := 0
		for _, l := range lines {
			if strings.TrimSpace(l) == want {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%s appears %d times, want 1", want, n)
		}
	}
}

func TestRun_ExistingConfig_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	if err := Run(dir); err != nil {
		t.Fatalf("first Run() failed: %v", err)
	}

	err := Run(dir)
	if err == nil {
		t.Fatal("expected error on second Run(), got nil")
	}
	if !strings.Contains(err.Error(), "already initialized") {
		t.Errorf("error %q does not mention 'already initialized'", err.Error())
	}
}
