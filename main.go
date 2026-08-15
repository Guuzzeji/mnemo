package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Guuzzeji/ai-shared-memory/internal/chunking"
	"github.com/Guuzzeji/ai-shared-memory/internal/config"
	"github.com/Guuzzeji/ai-shared-memory/internal/docs"
	"github.com/Guuzzeji/ai-shared-memory/internal/embeddings"
	"github.com/Guuzzeji/ai-shared-memory/internal/index"
	"github.com/Guuzzeji/ai-shared-memory/internal/initcmd"
	"github.com/Guuzzeji/ai-shared-memory/internal/logstore"
	"github.com/Guuzzeji/ai-shared-memory/internal/mcp"
	"github.com/Guuzzeji/ai-shared-memory/internal/sync"
	"github.com/Guuzzeji/ai-shared-memory/internal/taxonomy"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	newEmbedder = embeddings.NewEmbedder
	serve       = func(ctx context.Context, srv *sdk.Server) error { return srv.Run(ctx, &sdk.StdioTransport{}) }
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mnemo", flag.ContinueOnError)
	fs.SetOutput(stderr)
	initFlag := fs.Bool("init", false, "scaffold .memory_config.yaml and .memory-mcp/ directory, then exit")
	reindex := fs.Bool("reindex", false, "wipe the index and replay the entire log")
	configPath := fs.String("config", ".memory_config.yaml", "path to config file")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *initFlag {
		if err := initcmd.Run("."); err != nil {
			fmt.Fprintln(stderr, "init:", err)
			return 1
		}
		return 0
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(stderr, "invalid config:", err)
		return 1
	}

	logPath := cfg.ResolvePath(cfg.System.LogFile)
	docsDir := cfg.ResolvePath(cfg.System.DocsDir)
	dbPath := cfg.ResolvePath(cfg.Database.Path)
	modelDir := filepath.Dir(cfg.ResolvePath(cfg.Embeddings.ModelPath))

	logger := newLogger(filepath.Join(filepath.Dir(logPath), "server.log"))

	logStore, err := logstore.New(logPath)
	if err != nil {
		fmt.Fprintln(stderr, "open log:", err)
		return 1
	}
	docsStore, err := docs.New(docsDir, cfg.Taxonomy.AllowedCategories)
	if err != nil {
		fmt.Fprintln(stderr, "open docs:", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	baseURL := fmt.Sprintf("https://huggingface.co/%s/resolve/main", cfg.Embeddings.ModelRepo)
	if err := embeddings.EnsureModelDir(modelDir, baseURL, []embeddings.ModelFile{
		{LocalName: "model.onnx", RemotePath: "onnx/model.onnx", SHA256: cfg.Embeddings.ModelSHA256},
		{LocalName: "model.onnx_data", RemotePath: "onnx/model.onnx_data"},
		{LocalName: "tokenizer.json", RemotePath: "tokenizer.json"},
		{LocalName: "config.json", RemotePath: "config.json"},
	}); err != nil {
		logger.Warn("model download failed", "error", err)
	}

	embedder, err := newEmbedder(ctx, modelDir, cfg.Embeddings.Dimensions)
	if err != nil {
		if *reindex {
			fmt.Fprintln(stderr, "model unavailable (required for --reindex):", err)
			return 1
		}
		logger.Warn("embedding model unavailable; starting in degraded mode", "error", err)
	}

	var counter chunking.TokenCounter = embedder
	if embedder == nil {
		counter = fallbackCounter{}
	}
	chunker := chunking.New(cfg.Embeddings.Chunking.SplitOn, cfg.Embeddings.Chunking.MaxChunkTokens, cfg.Embeddings.Chunking.OverlapTokens, counter)

	idx, err := index.Open(dbPath, cfg.Embeddings.Dimensions)
	if err != nil {
		fmt.Fprintln(stderr, "open index:", err)
		return 1
	}
	defer idx.Close()

	var syncer *sync.Syncer
	if embedder != nil {
		syncer = &sync.Syncer{
			Log:      logStore,
			Docs:     docsStore,
			Chunker:  chunker,
			Embedder: embedder,
			Index:    idx,
			ModelID:  fmt.Sprintf("%s-%d", filepath.Base(modelDir), cfg.Embeddings.Dimensions),
		}
		if *reindex {
			if err := syncer.Reindex(ctx); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			logger.Info("reindex complete")
			return 0
		}
		if err := syncer.Sync(ctx); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		logger.Info("sync complete")
	}

	srv, err := mcp.NewServer(mcp.Deps{
		Config:   cfg,
		Log:      logStore,
		Docs:     docsStore,
		Index:    idx,
		Syncer:   syncer,
		Taxonomy: taxonomy.New(cfg.Taxonomy.AllowedCategories, cfg.Taxonomy.KeyTerms),
		Embedder: embedder,
	})
	if err != nil {
		fmt.Fprintln(stderr, "mcp server:", err)
		return 1
	}

	logger.Info("mnemo serving over stdio")
	if err := serve(ctx, srv); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(stderr, "mcp:", err)
		return 1
	}
	return 0
}

type fallbackCounter struct{}

func (fallbackCounter) CountTokens(text string) int { return len(text) / 4 }

func newLogger(path string) *slog.Logger {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return slog.New(slog.NewTextHandler(f, nil))
}
