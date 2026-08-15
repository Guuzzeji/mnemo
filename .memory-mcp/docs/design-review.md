---
id: design-review
status: active
tags:
    - memory
    - mcp
    - embeddings
    - index
---
# Design Review — Resolved Decisions

## Summary

Full design review of project-idea.md completed. Rejected LiteRT-LM (no Go bindings, no embedding symbols in prebuilt C API, gated model), sqlite-vector (Elastic 2.0 license trap, no Go bindings), and Turso (cloud dependency). Final stack is fully local and zero-CGO.

## Key Decisions

- **Embeddings:** EmbeddingGemma 300M ONNX from `onnx-community/embeddinggemma-300m-ONNX` (ungated) via `knights-analytics/hugot` pure-Go backend. 768-dim native, auto-download + SHA256 pin, gitignored.
- **Vector store:** `modernc.org/sqlite` + `modernc.org/sqlite/vec` (sqlite-vec, MIT/Apache). Local gitignored SQLite rebuilt from JSONL log. Exact scan `vec_distance_cosine` (~2-5ms @ 10k chunks); no ANN index needed.
- **Sync:** Position-based via `last_synced_log_id` (UUIDv7, monotonic) — immune to clock skew. `--reindex` for full rebuild.
- **IDs:** Server-generated UUIDv7 `log_id`; `author` = git user.name. Chunks keyed `sha256(doc_id+chunk_index)`.
- **MCP:** Official `modelcontextprotocol/go-sdk`, stdio transport. Tools: `semantic_search`, `append_memory`, `get_memory`, `list_memories`. MarkdownPatch restricted to `create` + `append_section` ops.
- **Taxonomy:** `allowed_categories` = closed write-time tag set; `key_terms` = query-time synonym expansion.
- **Ops:** Fail-fast config + `--init` scaffold. Zero CGO, goreleaser per-OS binaries. Logs to file (MCP logging deprecated in 2026-07-28 spec).