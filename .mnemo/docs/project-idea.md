---
id: project-idea
status: active
tags:
    - memory
    - mcp
    - embeddings
    - index
    - build
---
# Project Idea — Mnemo (ai-shared-memory)

## Summary

Single-binary Go MCP server acting as a Git-native AI memory framework. Fully local (no cloud): Git-tracked JSONL log + Markdown docs are the source of truth; a gitignored local SQLite vec0 index provides semantic search, rebuilt from the log. Zero CGO. Driven by `.memory_config.yaml` for portability across repos.

## Key Decisions

- **MCP:** official `modelcontextprotocol/go-sdk`, stdio transport. No MCP logging feature (deprecated 2026-07-28 spec) — log to file.
- **Embeddings:** EmbeddingGemma 300M ONNX (`onnx-community/embeddinggemma-300m-ONNX`, ungated) via `hugot` pure-Go gomlx backend. 768-dim native (Matryoshka 512/256/128). Auto-download + SHA256 pin, gitignored.
- **Vector store:** `modernc.org/sqlite` + `modernc.org/sqlite/vec` (sqlite-vec, MIT/Apache, pure Go). Local gitignored SQLite rebuilt from JSONL log. Exact scan `vec_distance_cosine` (~2-5ms @ 10k chunks); no ANN index needed.
- **Sync:** position-based via `last_synced_log_id` (UUIDv7, monotonic) — immune to clock skew. `--reindex` for full rebuild.
- **IDs:** server-generated UUIDv7 `log_id`; `author` = git user.name. Chunks keyed `sha256(doc_id+chunk_index)`.
- **Tools:** `semantic_search`, `append_memory`, `get_memory`, `list_memories`. MarkdownPatch restricted to `create` + `append_section` ops.
- **Taxonomy:** `allowed_categories` = closed write-time tag set; `key_terms` = query-time synonym expansion.
- **Ops:** fail-fast config + `--init` scaffold. Zero CGO, goreleaser per-OS binaries. Logs to file.

# Role Context

You are an expert Go backend engineer specializing in AI tooling, Model Context Protocol (MCP) servers, and vector database architectures.

Your task is to build a single-binary Go MCP server that acts as a Git-native AI memory framework. This framework balances structured machine-readability with human-readable, PR-friendly documentation. Everything is stored locally in the repository — no cloud resources — so every piece of shared memory can be reviewed by a human in the repo or in a PR before it reaches the wider team. It must be driven by a central `.memory_config.yaml` file to allow seamless portability across different repositories.

This project differentiates itself from the reference `@modelcontextprotocol/server-memory` (knowledge-graph, per-user, reference-grade) by being Git-native (team-shared, PR-reviewable), Markdown/ADR-shaped, and semantically searchable.

# Technical Stack

- **Language:** Pure Go. Must compile to a single executable binary per OS/arch with **zero CGO** (achievable: every dependency below is pure Go).
- **Protocol:** Model Context Protocol (MCP) over stdio, using the official SDK `github.com/modelcontextprotocol/go-sdk` (`mcp.StdioTransport{}`). Do NOT use the MCP logging feature (deprecated in the 2026-07-28 spec) — log to a local file instead.
- **Vector Store:** Local SQLite via `modernc.org/sqlite` + `modernc.org/sqlite/vec` (sqlite-vec extension bundled, MIT/Apache-2.0, pure Go, auto-registered). The database file lives inside the repo, is `.gitignore`d, and is rebuilt from the Git-tracked log. Exact-scan KNN (`ORDER BY vec_distance_cosine(...) LIMIT k`) — ~2–5ms at 10k×768-dim chunks; no ANN index needed at this scale.
- **Local Embeddings:** Google's **EmbeddingGemma 300M** in ONNX format (`onnx-community/embeddinggemma-300m-ONNX` — ungated official mirror, quantized variants available), executed by `github.com/knights-analytics/hugot` with the pure-Go gomlx backend (`NewGoSession()` — zero CGO). Native output is 768 dimensions (Matryoshka truncation to 512/256/128 possible via config). The pure-Go backend is 5–20x slower than C runtimes — irrelevant for embedding short texts at MCP call rates.

# The Central Configuration (`.memory_config.yaml`)

To make this framework repository-agnostic, the Go server reads `.memory_config.yaml` from the project root. This file controls file paths, enforces the taxonomy, and tunes the embedding pipeline. It is safe to commit (contains no secrets).

Implement a Go struct using `gopkg.in/yaml.v3` to parse this structure:

```yaml
system:
  # Pathing & File Definitions
  log_file: "memory/memory_log.jsonl"
  docs_dir: "memory/docs"

database:
  # Local SQLite index. Ephemeral cache — rebuilt from the log after git pull.
  path: "memory/index.db"

embeddings:
  # ONNX model, auto-downloaded on first boot (pinned URL + SHA256 verified),
  # stored gitignored. Offline override: point model_path at a local file.
  model_repo: "onnx-community/embeddinggemma-300m-ONNX"
  model_path: "memory/models/embeddinggemma-300m.onnx"
  dimensions: 768 # must match model output; mismatch vs index → --reindex required
  search_top_k: 5
  chunking:
    split_on: ["## "] # Markdown header chunk boundaries
    max_chunk_tokens: 500 # counted with the model's own tokenizer (ships with ONNX repo)
    overlap_tokens: 50    # overlap when an oversized section must be split

taxonomy:
  # CLOSED tag set — validated on every write.
  allowed_categories:
    - backend
    - frontend
    - devops
    - architecture
  # QUERY-TIME synonym expansion — aliases map a search term to its canonical
  # term. These are NOT categories and are never written to entries.
  key_terms:
    "authentication": ["auth", "jwt", "session", "login"]
    "database": ["db", "sql", "sqlite", "schema"]
```

## Config Boot Rules

- **Missing config → fail fast** with a clear error pointing to `--init`. Never silently default paths (wrong-repo writes are the worst failure mode).
- `--init` CLI flag scaffolds: config file, `memory/` directories, and appends `memory/index.db` + `memory/models/` to `.gitignore`.
- **Validate everything at boot, reporting ALL errors at once**: paths resolvable, `dimensions` matches the loaded model's actual output, `allowed_categories` non-empty, `split_on` non-empty, `max_chunk_tokens` > 0.

# Core Architecture: The Hybrid Memory System

The local file system (dictated by the config) is the **Source of Truth** (tracked in Git, PR-reviewed). The local SQLite database is a disposable **Search Index**.

## 1. The Source of Truth (Git-Tracked)

**The Append Log** — JSONL event log at `system.log_file`, the audit trail. "Keep a Changelog" semantics; resilient to Git squash-and-merge (tracks files/PRs, never commit SHAs).

- Merge conflicts: accepted by design — JSONL conflicts are line-level; resolution recipe is "keep both sides, sort by timestamp." Document this; do not build CRDTs.
- Every entry carries a server-generated **UUIDv7 `log_id`** (time-ordered, unique, never supplied by the LLM). The `author` field is `git config user.name` with an env override. Timestamps are server-set UTC, for display/audit only — never for sync logic.

**The Knowledge Tree** — Markdown files in `system.docs_dir`, strict YAML frontmatter + ADR-style structure. Enforced on write and during boot sync; violations reject the operation with an explicit error.

- Required frontmatter: `id` (slug, unique across `docs_dir`), `tags` (⊆ `allowed_categories`), `status` ∈ `active|deprecated`.
- Required body sections: `## Summary`, `## Key Decisions`.

## 2. The Search Index (Local SQLite)

`database.path` — ephemeral semantic cache, safe to delete at any time (rebuilt from the log). Schema:

```sql
CREATE TABLE docs (
  id TEXT PRIMARY KEY,
  path TEXT NOT NULL,
  tags TEXT NOT NULL,        -- JSON array, ⊆ allowed_categories
  status TEXT NOT NULL,      -- active | deprecated
  updated_at TEXT NOT NULL
);

CREATE VIRTUAL TABLE chunks USING vec0(
  chunk_id TEXT PRIMARY KEY, -- sha256(doc_id + chunk_index): stable across re-embeds
  doc_id TEXT,
  chunk_index INTEGER,
  heading TEXT,
  content TEXT,
  embedding float[768]       -- dimensions from config
);

CREATE TABLE sync_state (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  last_synced_log_id TEXT NOT NULL,  -- UUIDv7 = sync position (monotonic, no clock skew)
  model_id TEXT NOT NULL,            -- detects model/dimension changes
  synced_at TEXT NOT NULL
);
```

Chunk IDs are position-based (`sha256(doc_id + chunk_index)`): stable across content edits (update in place), trailing chunks cleaned by count comparison. One DB per repo — no multi-tenancy, no `repo` column.

# Server Lifecycle & Features

## 1. The Boot Sequence (Delta Sync ETL)

On startup, after loading + validating `.memory_config.yaml`:

1. Ensure the ONNX model exists at `embeddings.model_path`; if missing, download from `embeddings.model_repo` and verify the pinned SHA256.
2. Open/create the SQLite DB at `database.path`. If `sync_state.model_id` mismatches the current model/dimensions → require `--reindex`.
3. Read `last_synced_log_id` from `sync_state` (position-based — immune to team clock skew).
4. Scan `system.log_file` for entries after that ID → identify affected `target_docs`.
5. Detect deletions: docs in the index but missing on disk → remove their rows/chunks.
6. **Chunk:** parse affected `.md` files, split on `embeddings.chunking.split_on`, enforce `max_chunk_tokens` with `overlap_tokens` using the model tokenizer.
7. **Embed:** run chunks through local EmbeddingGemma (hugot pure-Go session).
8. **Upsert:** idempotent writes keyed by stable `chunk_id`, then update `sync_state` — only after the batch commits, so a killed sync replays harmlessly.
9. `--reindex` flag: wipe tables, replay the entire log (used for model changes, schema migrations, corruption).

Degraded mode: if embedding/model load fails, the server still starts — `semantic_search` returns an explicit "index unavailable" error, while `append_memory` continues writing to the JSONL log (source of truth) and reindexes on next boot. Git files are never blocked by index availability. One server instance per repo (stdio = one client spawns one server); concurrent instances are unsupported in v1.

## 2. MCP Tools to Expose

### `semantic_search(query string, tags []string)`

1. Expand query terms through `taxonomy.key_terms` aliases (case-insensitive synonym expansion).
2. Generate the query embedding with local EmbeddingGemma (every call — milliseconds; no cache in v1).
3. Exact-scan query: `ORDER BY vec_distance_cosine(embedding, ?) LIMIT search_top_k`, filtered by `tags` ⊆ doc tags and `status = 'active'`.

### `append_memory(entry MemoryLogEntry, markdown_updates []MarkdownPatch)`

1. Normalize `entry.tags` through `key_terms` aliases, validate against `allowed_categories`. Unknown tags → reject with error listing valid categories and near-miss suggestions. Never silently drop or auto-create categories.
2. Server sets `log_id` (UUIDv7), `timestamp`, `author` — the LLM never supplies them.
3. Append the entry to `system.log_file`.
4. Apply markdown updates — **exactly two ops allowed** (the server owns all file layout; the LLM supplies content only — free-form LLM patching is the #1 corruption vector):
   - `{doc_id, op: "create", frontmatter, body}` — new doc, validates frontmatter + required sections.
   - `{doc_id, op: "append_section", heading, content}` — append to an existing doc.
5. Reindex affected docs inline (chunk → embed → upsert).
6. Deprecation flows through `action: "DEPRECATED"` + frontmatter `status: deprecated` — chunks become invisible to search. Physical file deletion is caught by boot sync (step 5 above). No separate delete tool.

### `get_memory(doc_id string)`

Return the full Markdown document — LLMs need exact reads, not just top-k fragments.

### `list_memories(tags []string, status string)`

List doc IDs, paths, tags, status — for orientation and PR review workflows.

# Data Structures

## A. The JSONL Event Log Struct

```go
type ActionType string

const (
    ActionAdded      ActionType = "ADDED"
    ActionChanged    ActionType = "CHANGED"
    ActionDeprecated ActionType = "DEPRECATED"
    ActionFixed      ActionType = "FIXED"
    ActionDecision   ActionType = "DECISION"
)

type MemoryLogEntry struct {
    LogID         string     `json:"log_id"`                  // server-generated UUIDv7 — sync position
    Timestamp     time.Time  `json:"timestamp"`               // server-set UTC — audit only
    Author        string     `json:"author"`                  // git config user.name (env override)
    Action        ActionType `json:"action"`
    TargetDocs    []string   `json:"target_docs"`
    Tags          []string   `json:"tags"`                    // validated ⊆ allowed_categories
    Summary       string     `json:"summary"`
    Reasoning     string     `json:"reasoning,omitempty"`
    AffectedFiles []string   `json:"affected_files,omitempty"` // squash-resilient linkage
    PRReference   string     `json:"pr_reference,omitempty"`
}
```

## B. The Markdown Template

```markdown
---
id: "unique_domain_slug"        # unique across docs_dir
tags: ["allowed", "tags"]       # ⊆ allowed_categories
status: "active"                # active | deprecated
---

# [Component Name]

## Summary
...

## Key Decisions
...
```

# Distribution & Ops

- **Single binary per OS/arch** via goreleaser (darwin/linux, amd64/arm64) + `go install`. Zero CGO throughout, so cross-compilation is trivial. End-user setup: install binary → point MCP client at it → `--init` in a repo → done. Model downloads itself on first boot.
- **Logging:** structured logs to a local file (stdout is reserved for the MCP protocol; stderr/file only). Log sync counts, embed latency, index errors. No telemetry in v1.
- **Testing:** (1) pure unit tests — config parse/validate, tag normalization, chunking, JSONL append; (2) ETL tests against a `file:` SQLite DB (no external services in CI); (3) one golden-file integration test — fixture `memory/` dir → boot → search returns the expected chunk.

# Resolved Stack

| Component | Choice |
|---|---|
| MCP | `modelcontextprotocol/go-sdk`, stdio |
| Embeddings | EmbeddingGemma 300M ONNX (`onnx-community/embeddinggemma-300m-ONNX`) via `hugot` pure-Go gomlx backend, 768-dim |
| Vector store | `modernc.org/sqlite` + `modernc.org/sqlite/vec`, local gitignored SQLite, rebuilt from log |
| Vector query | exact scan `vec_distance_cosine`, `LIMIT search_top_k` |
| Config | `gopkg.in/yaml.v3`, strict boot validation, fail-fast when missing, `--init` scaffold |
| IDs | UUIDv7 `log_id`; `author` = git user.name; chunks = `sha256(doc_id+chunk_index)` |
| Sync | position-based (`last_synced_log_id`), never timestamps; `--reindex` for full rebuild |
| Taxonomy | `allowed_categories` = closed write-time tag set; `key_terms` = query-time synonym expansion |
## ## Reindex MCP Tool
Added `reindex` MCP tool (2026-08-15): wipes the semantic index and replays the entire log — the agent-facing equivalent of the CLI `--reindex` flag. Returns `indexed_docs` count. Errors with "index unavailable: embedding model not loaded" in degraded mode. Full tool list is now: `semantic_search`, `append_memory`, `get_memory`, `list_memories`, `reindex`.