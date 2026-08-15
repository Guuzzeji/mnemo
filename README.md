# Mnemo

Git-native memory for AI agents. A single-binary MCP server that gives every
agent working in your repo a shared, searchable memory of decisions — stored
**inside the repository**, reviewed in PRs, and semantically searchable. No
cloud, no external database.

- **Source of truth:** a Git-tracked JSONL log (`.memory-mcp/memory_log.jsonl`)
- **Human-readable projection:** a Markdown tree (`.memory-mcp/docs/`)
- **Search index:** a disposable SQLite `vec0` index (`.memory-mcp/index.db`,
  gitignored, rebuilt from the log)
- **Embeddings:** local EmbeddingGemma 300M (ONNX, gitignored, downloads on
  first boot) — zero CGO, works offline after download

## Quick start

```sh
# 1. Build the server binary into your project (gitignored)
go build -o .memory-mcp/mnemo .

# 2. Scaffold config + .memory-mcp/ in your repo (appends gitignore entries)
.memory-mcp/mnemo --init

# 3. Register in your MCP client (project-scoped .mcp.json):
#    { "mcpServers": { "mnemo": { "command": ".memory-mcp/mnemo", "args": [] } } }

# 4. Run the MCP server over stdio
.memory-mcp/mnemo
```

Prefer a release binary or `go install github.com/Guuzzeji/ai-shared-memory@latest`
(installs as `ai-shared-memory`)? See [setup.md](setup.md).

On first boot the embedding model downloads from Hugging Face to
`.memory-mcp/models/`. If the model is unavailable the server starts in
degraded mode: `semantic_search` reports the index unavailable, while
`append_memory` still writes to the log.

## Documentation

| Doc                                          | Audience                                                       |
| -------------------------------------------- | -------------------------------------------------------------- |
| [`setup.md`](setup.md)                       | Humans — full setup, config reference, team workflow           |
| [`AGENT-SETUP.md`](AGENT-SETUP.md)           | AI agents — setup a project autonomously                       |
| [`AGENT-TEMPLATE.md`](AGENT-TEMPLATE.md)     | Teams — copy into a project so all agents use memory correctly |

## How memory is stored

| Path                           | Contents                  | Git     |
| ------------------------------ | ------------------------- | ------- |
| `.memory_config.yaml`          | Central config            | Commit  |
| `.memory-mcp/memory_log.jsonl` | Append-only audit log     | Commit  |
| `.memory-mcp/docs/`            | Markdown memory documents | Commit  |
| `.memory-mcp/index.db`         | Semantic search index     | Ignored |
| `.memory-mcp/index.db-shm`     | SQLite WAL shared memory  | Ignored |
| `.memory-mcp/index.db-wal`     | SQLite WAL journal        | Ignored |
| `.memory-mcp/mnemo`            | The executable binary     | Ignored |
| `.memory-mcp/models/`          | Embedding model           | Ignored |

`mnemo --init` appends the ignore entries to your `.gitignore` automatically
(including the WAL sidecars, the binary, and `.sisyphus/`).

## MCP tools

| Tool              | Description                                                                                                                                |
| ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `semantic_search` | Embed query, cosine-distance scan, `LIMIT k`, tag/status filters, alias expansion                                                          |
| `append_memory`   | Normalize/validate tags, server-set timestamps, append log, apply `create`/`append_section` markdown ops, inline reindex, deprecation flow |
| `get_memory`      | Read a memory doc by ID                                                                                                                    |
| `list_memories`   | List memory docs                                                                                                                           |

Logging goes to a local file (`.memory-mcp/server.log`), never to MCP — the
MCP logging feature is deprecated in the 2026-07-28 spec and stdout is
reserved for the stdio protocol.

## CLI

| Flag        | Description                                                                  |
| ----------- | ---------------------------------------------------------------------------- |
| `--init`    | Scaffold `.memory_config.yaml` and `.memory-mcp/`, update `.gitignore`, exit |
| `--reindex` | Wipe the index and replay the entire log                                     |
| `--config`  | Path to config file (default `.memory_config.yaml`)                          |

## Requirements

- Go 1.26+ (to build) — no CGO, cross-compilation trivial
- ~600 MB disk for the embedding model

## Build & test

```sh
CGO_ENABLED=0 go build -o .memory-mcp/mnemo .
CGO_ENABLED=0 go test ./...
```

Unit tests per package, ETL tests against a `file:` SQLite DB, and one
golden-file integration test (`internal/integration`) that boots the full
stack over an in-memory MCP transport.

## Release

```sh
goreleaser release --clean
```

Builds `mnemo` for darwin/linux, amd64/arm64, all with `CGO_ENABLED=0`.

## License

MIT
