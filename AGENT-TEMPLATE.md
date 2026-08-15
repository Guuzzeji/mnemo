# Agent Memory Rules — Mnemo

> This file tells every AI agent working in this repository how to use the
> project's shared memory. It is managed by the team — update it when the
> workflow changes. The source file in the Mnemo repo is `AGENT-TEMPLATE.md`;
> copy it into your project root as `agent.md` (or `AGENTS.md`
> if your tooling auto-loads that name).

---

## What this project remembers

This repository uses **Mnemo**, a Git-native memory server. Agents working here
have shared, searchable memory of decisions, architecture, and gotchas. The
memory lives **inside this repo**:

```
.mnemo/
├── memory_log.jsonl    # append-only audit log (source of truth)
└── docs/               # human-readable memory documents
```

The memory is committed and **reviewed in PRs like code** — write it so a human
teammate can understand it without you. Binary artifacts (`index.db` and its
`-shm`/`-wal` sidecars, `models/`, the `mnemo` binary) are gitignored and
never committed.

## Available MCP tools

| Tool | When to use it |
|------|----------------|
| `semantic_search(query, tags?)` | **Before starting any task.** Find prior decisions relevant to what you're about to do. |
| `get_memory(doc_id)` | Read a full memory document when a search snippet isn't enough. |
| `list_memories(tags?, status?)` | Orient yourself: what does this project know about? |
| `append_memory(entry, markdown_updates?)` | Record a decision, gotcha, or change. |

## Workflow — every task

1. **Search first.** Before implementing or investigating, call
   `semantic_search` with the relevant terms (e.g. the component, the problem
   domain). Act on what you find — do not duplicate past decisions.
2. **Work.** Implement/fix/investigate.
3. **Record.** After a meaningful outcome — a decision, a non-obvious fix, a
   changed design, a known gotcha — write it to memory with `append_memory`.
   "Meaningful" = a future agent would waste time without it. Not every commit.

## Tagging rules

- Only use categories defined in `.memory_config.yaml` →
  `taxonomy.allowed_categories`.
- **Never invent tags.** Unlisted tags are rejected; put them in the config
  first (human-reviewed change).
- Use the categories that best fit the memory's subject.

## Status rules

- New memory: `status: active`.
- Superseded/obsolete memory: mark `status: deprecated` via `append_memory`
  (action `DEPRECATED`) instead of deleting. Deprecated docs become invisible
  to search but stay in history for audit.
- `create` and `append_section` are the only document operations you may
  request. Do not hand-edit files in `.mnemo/docs/`.

## What agents must NEVER do

- ❌ Hand-edit `.mnemo/memory_log.jsonl` or `.mnemo/docs/*` with file
  tools. Memory is written through the MCP tools only — it validates tags,
  sets server timestamps, and keeps log + docs consistent.
- ❌ Commit `.mnemo/index.db`, `.mnemo/index.db-shm`,
  `.mnemo/index.db-wal`, `.mnemo/models/`, or the `mnemo`
  binary. They are gitignored; `git status` must never show them staged.
- ❌ Skip `semantic_search` before large or risky work.
- ❌ Delete or rewrite memory docs directly. Use deprecation, or ask a human.

## Writing quality

Memory is written for humans who review PRs and for agents who search later:

- `summary`: one or two sentences, concrete ("Added retry with backoff to the
  payment webhook", not "Did some work").
- `reasoning`: why this choice — the context a future agent needs to avoid
  re-litigating it.
- `target_docs` / `affected_files`: name the doc and files involved, so the
  entry links to the code.

## If Mnemo is unavailable

- Server not running → the client may not expose the tools. Proceed with your
  task and note in your report that memory tools were unavailable.
- Semantic search reports "index unavailable" → the embedding model failed to
  load; the server is in degraded mode. You can still write memory. Tell a
  human: run `mnemo --reindex` or fix the model download.

## Project-specific configuration

- **Project:** `Mnemo` (ai-shared-memory)
- **Allowed categories:** `[memory, mcp, embeddings, index, cli, build]` (from
  `.memory_config.yaml`)
- **Anything else agents must know:** Mnemo is a Go single-binary MCP memory
  server. Record decisions about the MCP tool surface, the embedding pipeline
  (EmbeddingGemma ONNX), the SQLite vec0 index, the JSONL log format, CLI
  flags, and build/release (goreleaser) under the matching category. This repo
  is also the reference implementation — memory written here doubles as the
  project's own docs.