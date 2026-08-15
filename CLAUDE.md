# CLAUDE.md — Agent Memory Rules (Mnemo)

> Tells every AI agent in this repo how to use the project's shared memory.
> Team-managed; update when the workflow changes.

## What this project remembers

This repo uses **Mnemo**, a Git-native MCP memory server. Memory is shared,
searchable, and lives **inside this repo**:

```
.memory-mcp/
├── memory_log.jsonl    # append-only audit log (source of truth)
└── docs/               # human-readable memory documents
```

Committed and **reviewed in PRs like code**. Binary artifacts (`index.db`,
`index.db-shm`, `index.db-wal`, `models/`) are gitignored, never committed.

## MCP tools

| Tool | When |
|------|------|
| `semantic_search(query, tags?)` | **Before any task.** Find prior decisions relevant to what you're doing. |
| `get_memory(doc_id)` | Read a full doc when a snippet isn't enough. |
| `list_memories(tags?, status?)` | Orient: what does this project know? |
| `append_memory(entry, markdown_updates?)` | Record a decision, gotcha, or change. |

## Workflow — every task

1. **Search first.** Call `semantic_search` before implementing/investigating.
   Act on findings — don't duplicate past decisions.
2. **Work.** Implement/fix/investigate.
3. **Record.** After a meaningful outcome (decision, non-obvious fix, changed
   design, gotcha), `append_memory`. "Meaningful" = a future agent would waste
   time without it. Not every commit.

## Tagging rules

- Only categories from `.memory_config.yaml` → `taxonomy.allowed_categories`.
- **Never invent tags.** Unlisted tags are rejected; add to config first
  (human-reviewed change).

## Status rules

- New memory: `status: active`.
- Obsolete: mark `status: deprecated` via `append_memory` (action `DEPRECATED`)
  instead of deleting. Deprecated docs vanish from search, stay for audit.
- Only `create` and `append_section` doc ops. Do not hand-edit
  `.memory-mcp/docs/`.

## Never do

- ❌ Hand-edit `.memory-mcp/memory_log.jsonl` or `.memory-mcp/docs/*` with file
  tools. Write through MCP tools only (validates tags, sets timestamps, keeps
  log + docs consistent).
- ❌ Commit `.memory-mcp/index.db`, `.memory-mcp/index.db-shm`,
  `.memory-mcp/index.db-wal`, `.memory-mcp/models/`, or the `mnemo`
  binary — gitignored; `git status` must never show them staged.
- ❌ Skip `semantic_search` before large or risky work.
- ❌ Delete/rewrite memory docs directly. Deprecate, or ask a human.

## Writing quality

Written for humans reviewing PRs and agents searching later:
- `summary`: 1–2 concrete sentences ("Added retry with backoff to the payment
  webhook", not "Did some work").
- `reasoning`: why this choice — context a future agent needs to avoid
  re-litigating it.
- `target_docs` / `affected_files`: name the doc and files involved.

## If Mnemo is unavailable

- Server not running → tools may be absent. Proceed, note in your report that
  memory tools were unavailable.
- "Index unavailable" → embedding model failed to load; degraded mode. Can
  still write memory. Tell a human: run `mnemo --reindex` or fix the model.

## Project config

- **Project:** Mnemo (ai-shared-memory)
- **Allowed categories:** `[memory, mcp, embeddings, index, cli, build]`
- **Stack:** Go single-binary MCP memory server. EmbeddingGemma ONNX via hugot
  pure-Go (768-dim), SQLite vec0 index (`modernc.org/sqlite/vec`), JSONL log,
  goreleaser builds. Zero CGO. Record decisions under the matching category.
  This repo is the reference implementation — memory here doubles as the
  project's own docs.