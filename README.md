<p align="center">

<img src="./assets/banner.jpg">

</p>

<h1 align="center">Mnemo</h1>

Git-native memory for AI agents. A single-binary MCP server that gives every
agent working in your repo a shared, searchable memory of decisions — stored
**inside the repository**, reviewed in PRs, and semantically searchable. No
cloud, no external database.

- **Source of truth:** a Git-tracked JSONL log (`.mnemo/memory_log.jsonl`)
- **Human-readable projection:** a Markdown tree (`.mnemo/docs/`)
- **Search index:** a disposable SQLite `vec0` index (`.mnemo/index.db`,
  gitignored, rebuilt from the log)
- **Embeddings:** local EmbeddingGemma 300M (ONNX, gitignored, downloads on
  first boot) — zero CGO, works offline after download

## Quick start

```sh
# 1. Build the server binary into your project (gitignored)
go build -o .mnemo/mnemo .

# 2. Scaffold config + .mnemo/ in your repo (appends gitignore entries)
.mnemo/mnemo --init

# 3. Register in your MCP client (project-scoped .mcp.json):
#    { "mcpServers": { "mnemo": { "command": ".mnemo/mnemo", "args": [] } } }

# 4. Run the MCP server over stdio
.mnemo/mnemo
```

Prefer a release binary or `go install github.com/Guuzzeji/ai-shared-memory@latest`
(installs as `ai-shared-memory`)? See [setup.md](setup.md).

On first boot the embedding model downloads from Hugging Face to
`.mnemo/models/`. If the model is unavailable the server starts in
degraded mode: `semantic_search` reports the index unavailable, while
`append_memory` still writes to the log.

## Documentation

| Doc                                      | Audience                                                       |
| ---------------------------------------- | -------------------------------------------------------------- |
| [`setup.md`](setup.md)                   | Humans — full setup, config reference, team workflow           |
| [`AGENT-SETUP.md`](AGENT-SETUP.md)       | AI agents — setup a project autonomously                       |
| [`AGENT-TEMPLATE.md`](AGENT-TEMPLATE.md) | Teams — copy into a project so all agents use memory correctly |

## How memory is stored

| Path                           | Contents                  | Git     |
| ------------------------------ | ------------------------- | ------- |
| `.memory_config.yaml`          | Central config            | Commit  |
| `.mnemo/memory_log.jsonl` | Append-only audit log     | Commit  |
| `.mnemo/docs/`            | Markdown memory documents | Commit  |
| `.mnemo/index.db`         | Semantic search index     | Ignored |
| `.mnemo/index.db-shm`     | SQLite WAL shared memory  | Ignored |
| `.mnemo/index.db-wal`     | SQLite WAL journal        | Ignored |
| `.mnemo/mnemo`            | The executable binary     | Ignored |
| `.mnemo/models/`          | Embedding model           | Ignored |

`mnemo --init` appends the ignore entries to your `.gitignore` automatically
(including the WAL sidecars, the binary, and `.sisyphus/`).

## MCP tools

| Tool              | Description                                                                                                                                |
| ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `semantic_search` | Embed query, cosine-distance scan, `LIMIT k`, tag/status filters, alias expansion                                                          |
| `append_memory`   | Normalize/validate tags, server-set timestamps, append log, apply `create`/`append_section` markdown ops, inline reindex, deprecation flow |
| `get_memory`      | Read a memory doc by ID                                                                                                                    |
| `list_memories`   | List memory docs                                                                                                                           |
| `reindex`         | Wipe the semantic index and rebuild it by replaying the entire memory log                                                                  |

Logging goes to a local file (`.mnemo/server.log`), never to MCP — the
MCP logging feature is deprecated in the 2026-07-28 spec and stdout is
reserved for the stdio protocol.

## CLI

| Flag        | Description                                                                  |
| ----------- | ---------------------------------------------------------------------------- |
| `--init`    | Scaffold `.memory_config.yaml` and `.mnemo/`, update `.gitignore`, exit |
| `--reindex` | Wipe the index and replay the entire log                                     |
| `--config`  | Path to config file (default `.memory_config.yaml`)                          |

## Requirements

- Go 1.26+ (to build) — no CGO, cross-compilation trivial
- ~600 MB disk for the embedding model

## Build & test

```sh
CGO_ENABLED=0 go build -o .mnemo/mnemo .
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
