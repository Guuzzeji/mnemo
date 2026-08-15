---
id: embeddinggemma-onnx-gomlx
status: active
tags:
    - embeddings
    - mcp
    - build
---
## Summary

EmbeddingGemma 300M ONNX with gomlx/hugot: use the fp32 `model.onnx` + `model.onnx_data` variant (or `model_quantized.onnx`, int8 + DequantizeLinear). Do NOT use q4 or fp16 variants.

## Key Decisions

- onnx-gomlx supports external data via `FilesReader` (`filepath.Join(baseDir, location)`); hugot calls `WithBaseDir(model.Path)`, so `model.onnx_data` must sit next to the `.onnx` with the exact name referenced in the graph.
- q4 variants use `MatMulNBits`/`GatherBlockQuantized`/INT4 ops + dtype NOT in onnx-gomlx (`dtypeForONNX` has no INT4; janpfeifer confirmed no 4-bit support in GoMLX).
- fp16 variant is officially unsupported by the model authors (activation overflow) and mixes FLOAT16/FLOAT without `AllowDTypePromotion`.
- `model_quantized.onnx` uses DequantizeLinear + INT8 (supported) with all-FLOAT compute.
- fp32 `model.onnx` ops are all supported (RotaryEmbedding/MultiHeadAttention/SimplifiedLayerNormalization implemented for HF optimum export format).

## Files

- `onnx/model.onnx` (479 KB graph) + `onnx/model.onnx_data` (1.2 GB fp32 weights)
- `tokenizer.json`, `config.json` at repo root
- Download all four into `.memory-mcp/models/`

## Download Fix
## Download Fix (2026-08-15)

- `cmd/mnemo/main.go` now downloads all four required files via `EnsureModelDir`: `onnx/model.onnx` (SHA256-pinned), `onnx/model.onnx_data`, `tokenizer.json`, `config.json`.
- `internal/embeddings/download.go` gained `EnsureModelDir` (multi-file, `errors.Join` partial failures) and `os.MkdirAll(modelDir)` — the missing dir caused rename failures on first boot.
- Gotcha: a memory entry logged via MCP without its markdown doc breaks startup sync (`read doc X: not found`). Always pass `markdown_updates` with `create` (frontmatter + body with `## Summary` and `## Key Decisions`) when logging a CREATE.
