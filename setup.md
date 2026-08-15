# Mnemo — Setup Guide (for Humans)

**Mnemo** is a single-binary MCP memory server for AI agents. It gives every
agent working in your repository a shared, searchable memory of decisions,
architecture, and gotchas — stored **inside the repo**, reviewed in PRs, and
semantically searchable.

This guide is for humans. Agents setting themselves up should read
[`AGENT-SETUP.md`](AGENT-SETUP.md) instead.

---

## How Mnemo stores memory

Everything lives in your repo. No cloud, no external database.

| Path | Contents | Git |
|------|----------|-----|
| `.memory_config.yaml` | Central config (paths, taxonomy, model) | **Commit** |
| `.mnemo/memory_log.jsonl` | Source of truth — append-only audit log | **Commit** |
| `.mnemo/docs/` | Human-readable Markdown projection | **Commit** |
| `.mnemo/index.db` | Disposable semantic search index | **Ignore** (rebuildable) |
| `.mnemo/index.db-shm` | SQLite WAL shared memory | **Ignore** |
| `.mnemo/index.db-wal` | SQLite WAL journal | **Ignore** |
| `.mnemo/mnemo` | The executable binary | **Ignore** |
| `.mnemo/models/` | Downloaded embedding model | **Ignore** (redownloads) |

The JSONL log and Markdown docs are the memory — they get committed and
reviewed like any other code. The SQLite index and the embedding model are
binary artifacts: gitignored, safe to delete, rebuilt automatically. `mnemo --init`
appends the ignore entries to your `.gitignore` for you.

---

## Prerequisites

- **Go 1.26+** (to build from source), or a prebuilt binary from the
  [releases page](https://github.com/Guuzzeji/ai-shared-memory/releases)
- An MCP-capable AI client (Claude Code, Cursor, VS Code, Roo, etc.)
- ~600 MB free disk for the embedding model (downloaded on first boot)

---

## 1. Install

```sh
# From source (installs as the binary "ai-shared-memory")
go install github.com/Guuzzeji/ai-shared-memory@latest

# Or download the release binary for your OS/arch and put it on your PATH
```

You can also keep the binary inside the project instead of on PATH — the
default location is `.mnemo/mnemo` (gitignored):

```sh
go build -o .mnemo/mnemo .
```

The project-local build yields a binary named `mnemo`; `go install` yields
`ai-shared-memory` (the module base name). The project-local build is the
recommended flow.

Verify:

```sh
ai-shared-memory --help   # or: ./.mnemo/mnemo --help
```

---

## 2. Initialize a repo

From your project root:

```sh
mnemo --init
```

This creates:

- `.memory_config.yaml` — edit it to fit your project (categories, key terms)
- `.mnemo/` and `.mnemo/docs/`
- Appends `.mnemo/index.db`, `.mnemo/index.db-shm`,
  `.mnemo/index.db-wal`, `.mnemo/mnemo`, `.mnemo/models/`,
  `.sisyphus/` to `.gitignore`

If you keep the binary in the project, save it as `.mnemo/mnemo` (already
ignored): `go build -o .mnemo/mnemo .` or copy the release binary there.

Commit the config, the `.mnemo/` directory, and the `.gitignore` changes.
`--init` is idempotent and safe to re-run (it refuses to overwrite an existing
config).

> **New team?** Copy [`AGENT-TEMPLATE.md`](AGENT-TEMPLATE.md) into your repo
> root as `agent.md` (or `AGENTS.md`) so every AI agent that works on the
> project knows how to use Mnemo. See [Team workflow](#5-team-workflow).

---

## 3. Register the MCP server in your client

Point your AI client at the `mnemo` binary. Standard stdio MCP config:

```json
{
  "mcpServers": {
    "mnemo": {
      "command": "mnemo",
      "args": []
    }
  }
}
```

Claude Code:

```sh
claude mcp add --scope project mnemo -- mnemo
```

For a locally built binary, use the absolute path (e.g. `/path/to/mnemo`).

---

## 4. First boot

The first time `mnemo` runs, it downloads the embedding model
(EmbeddingGemma 300M, ~600 MB) from Hugging Face to `.mnemo/models/` and
builds the search index from your log.

- Model download fails (offline, no disk)? Mnemo still starts in **degraded
  mode** — agents can still write memory; only semantic search is unavailable.
- The index is always safe to delete; it rebuilds from the log on next boot.

---

## 5. Team workflow

Mnemo memory is **Git-native**: agents write to the log and docs, you review
and merge the changes in PRs.

1. Put [`AGENT-TEMPLATE.md`](AGENT-TEMPLATE.md) in your repo root as
   `agent.md` (or `AGENTS.md` if your tools auto-load that name).
2. Agents read existing memory before working and record decisions after.
3. Memory changes flow through normal commits/PRs — human review before it
   reaches the team.
4. Merge conflicts in `memory_log.jsonl`? Keep both sides, sort by timestamp
   — the log is append-only by design.

### MCP tools agents get

| Tool | Purpose |
|------|---------|
| `semantic_search` | Find relevant past decisions (`query`, optional `tags`) |
| `append_memory` | Record a decision/gotcha/change |
| `get_memory` | Read a full memory document |
| `list_memories` | Orient: list all docs, filter by tag/status |

---

## Config reference

```yaml
system:
  log_file: ".mnemo/memory_log.jsonl"   # source of truth
  docs_dir: ".mnemo/docs"               # markdown projection
database:
  path: ".mnemo/index.db"               # disposable vec0 index
embeddings:
  model_repo: "onnx-community/embeddinggemma-300m-ONNX"
  model_path: ".mnemo/models/embeddinggemma-300m.onnx"
  model_sha256: ""                    # optional pin; empty = skip verify
  dimensions: 768
  search_top_k: 5
  chunking:
    split_on: ["## "]
    max_chunk_tokens: 500
    overlap_tokens: 50
taxonomy:
  allowed_categories: [backend, frontend, devops, architecture]
  key_terms:
    authentication: [auth, jwt, session, login]
```

- `allowed_categories` is a **closed set** — agents can only tag memory with
  these. Add categories that fit your project.
- `key_terms` are query-time synonyms (e.g. searching "jwt" also matches
  "authentication").

---

## CLI

| Flag | Description |
|------|-------------|
| `--init` | Scaffold `.memory_config.yaml` + `.mnemo/`, update `.gitignore`, exit |
| `--reindex` | Wipe the index and replay the entire log |
| `--config` | Path to config file (default `.memory_config.yaml`) |

## Troubleshooting

| Problem | Fix |
|---------|-----|
| `config file not found` | Run `mnemo --init` |
| Semantic search says index unavailable | Embedding model failed to load; check disk space / network, then restart |
| Index corrupted or model changed | `mnemo --reindex` |
| Agent can't write tags | Tag not in `allowed_categories`; add it to config |