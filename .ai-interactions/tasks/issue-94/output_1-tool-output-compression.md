# Issue 94: Lossless Tool Output Compression

## Goal

After a built-in tool executes in Keen Code, transform its JSON output into a token-cheaper representation before the result is sent back to the LLM. The transformation must be **lossless**: every fact the model needs is preserved, and the original remains available in `TurnMemory.RawOutput` for internal use. MCP tool results are excluded because they are already handled by the existing MCP cache.

## Non-goals

- No lossy sampling or row dropping for this iteration.
- No embedding/tokenizer-based compression; keep it deterministic and cheap.
- No generic gzip/base64 wrapping that would make content opaque to the model.



## General gating rule

Only compress when the estimated token saving exceeds ~15% and the output exceeds a minimum token threshold (suggested 200 tokens). Use the existing `estimateJSONTokenCount` helper. If compression does not meet the threshold, return the original output unchanged.

## Per-tool compression plans



### `read_file`

Current output shape:

```json
{
  "content": "1:a3f|package tools\n2:811|\n3:c57|import (\n...",
  "bytes_read": 2048,
  "lines_read": 42,
  "total_lines": 500,
  "truncated": true
}
```

The anchored `N:HASH|line` format is already near-optimal for code. The hash is required by `edit_file`, so keep it for strict losslessness.

**Lossless wins:**

- Remove `lines_read` because it equals the number of lines in `content`.
- Remove `bytes_read`; not needed by the LLM for reasoning.
- Omit `truncated` when it is `false`.

Compressed:

```json
{
  "content": "1:a3f|package tools\n2:811|\n3:c57|import (\n...",
  "total_lines": 500,
  "truncated": true
}
```

> Optional future enhancement (behind a flag): strip hashes to produce `N|line`, re-reading with hashes only when the LLM wants to edit. Not enabled by default.

---



### `grep`

Current output repeats field names and full paths for every match:

```json
{
  "matches": [
    {"file": "/Users/mchow/proj/internal/llm/context_reducer.go", "line_number": 18, "line": "const removedToolResultPlaceholder = \"Tool result removed to fit context.\"", "line_hash": "978"},
    {"file": "/Users/mchow/proj/internal/llm/context_reducer.go", "line_number": 19, "line": "const contextWindowExceededError = \"context exceeds model window after removing tool results\"", "line_hash": "c04"},
    {"file": "/Users/mchow/proj/internal/llm/message.go", "line_number": 108, "line": "StreamEventTypeAutoCompactionStarted StreamEventType = \"auto_compaction_started\"", "line_hash": "699"}
  ]
}
```

**Compression strategy:**

- Extract the longest common directory prefix.
- Group matches by file.
- Replace repeated object keys with positional arrays: `[line_number, line_hash, line]`.

Compressed:

```json
{
  "_fmt": "grep-v1",
  "common_prefix": "/Users/mchow/proj/",
  "matches": {
    "internal/llm/context_reducer.go": [
      [18, "978", "const removedToolResultPlaceholder = \"Tool result removed to fit context.\""],
      [19, "c04", "const contextWindowExceededError = \"context exceeds model window after removing tool results\""]
    ],
    "internal/llm/message.go": [
      [108, "699", "StreamEventTypeAutoCompactionStarted StreamEventType = \"auto_compaction_started\""]
    ]
  }
}
```

Reconstruction rule: prepend `common_prefix` to each relative path; each row is `[line_number, line_hash, line]`.

Expected savings: 30–40% for many matches across multiple files.

---



### `glob`

Current output:

```json
{
  "files": [
    "/Users/mchow/proj/internal/tools/read_file.go",
    "/Users/mchow/proj/internal/tools/grep.go",
    "/Users/mchow/proj/internal/tools/glob.go",
    "/Users/mchow/proj/internal/llm/context_reducer.go"
  ]
}
```

**Compression strategy:**

- Extract the longest common prefix.

Compressed:

```json
{
  "_fmt": "glob-v1",
  "common_prefix": "/Users/mchow/proj/",
  "files": [
    "internal/tools/read_file.go",
    "internal/tools/grep.go",
    "internal/tools/glob.go",
    "internal/llm/context_reducer.go"
  ]
}
```

Expected savings: 20–30% for deep monorepo paths.

---



### `bash`

`bash` output is the most variable, so use content-type detection on `stdout`/`stderr`.

Base shape:

```json
{
  "exit_code": 0,
  "stdout": "...",
  "stderr": "",
  "truncated": false
}
```



#### 4a. JSON array of objects → CSV format

Before:

```json
{
  "exit_code": 0,
  "stdout": "[\n  {\"name\": \"context_reducer.go\", \"lines\": 412, \"package\": \"llm\"},\n  {\"name\": \"message.go\", \"lines\": 145, \"package\": \"llm\"},\n  {\"name\": \"tool.go\", \"lines\": 110, \"package\": \"tools\"}\n]"
}
```

After:

```json
{
  "exit_code": 0,
  "stdout": {
    "_fmt": "csv",
    "header": ["name", "lines", "package"],
    "rows": [
      ["context_reducer.go", 412, "llm"],
      ["message.go", 145, "llm"],
      ["tool.go", 110, "tools"]
    ]
  }
}
```

Reconstruction: `header` + `rows` rebuilds the original JSON array.

Expected savings: 40–50% for 50+ rows with several columns.

#### 4b. File listings → glob-v1 format

Before:

```text
internal/llm/context_reducer.go
internal/llm/message.go
internal/tools/tool.go
```

After:

```json
{
  "_fmt": "glob-v1",
  "common_prefix": "internal/",
  "files": ["llm/context_reducer.go", "llm/message.go", "tools/tool.go"]
}
```



#### 4c. Logs / repeated lines → run-length encoding

Before:

```text
INFO: request /health
INFO: request /health
INFO: request /health
ERROR: db timeout
INFO: request /ready
INFO: request /ready
```

After:

```json
{
  "_fmt": "rle-lines",
  "lines": [
    ["INFO: request /health", 3],
    ["ERROR: db timeout", 1],
    ["INFO: request /ready", 2]
  ]
}
```

Reconstruction: expand each `[line, count]` pair back into `count` repetitions.

Expected savings: large for repetitive logs.

#### 4d. Already truncated output

If `truncated: true`, `stdout_file` already holds the full output. The inline preview is bounded; leave it unchanged.

---



### `web_fetch`

Current output:

```json
{
  "status_code": 200,
  "content": "...",
  "truncated": true,
  "artifact_path": "/Users/mchow/.keen/keen-web-fetch-xxx.txt"
}
```

When `truncated` is true, the inline content is already a head+tail preview. Further lossless wins are marginal. Leave as-is for strict losslessness.

Minor optional future win: collapse runs of more than two newlines to two newlines.

---



### `ask_user`

Output is small, e.g.:

```json
{"answers": ["yes"], "cancelled": false}
```

Skip compression.

---



### `write_file` / `edit_file`

Outputs are status maps, usually <50 tokens, e.g.:

```json
{"status": "success"}
```

Skip compression.

---



### `call_mcp_tool`

MCP results are already handled by the robust MCP cache. Represent large cached results as a pointer:

```json
{
  "_fmt": "mcp-cached",
  "server": "my-mcp-server",
  "tool": "search",
  "cache_key": "sha256:abc123...",
  "summary": "42 results"
}
```

This avoids repeating large MCP payloads in-context because the cache can serve the original on demand.

---



### `delegate_task`

A delegate result is often a nested report. Apply the same rules recursively:

- Top-level arrays of objects → `csv` format.
- File paths → `glob-v1` prefix extraction.
- Long plain-text sections → `rle-lines` if repetitive.

Before:

```json
{
  "status": "success",
  "findings": [
    {"file": "/Users/mchow/proj/internal/a.go", "issue": "unused import", "line": 5},
    {"file": "/Users/mchow/proj/internal/b.go", "issue": "unused import", "line": 12}
  ]
}
```

After:

```json
{
  "status": "success",
  "findings": {
    "_fmt": "csv",
    "common_prefix": "/Users/mchow/proj/",
    "header": ["file", "issue", "line"],
    "rows": [
      ["internal/a.go", "unused import", 5],
      ["internal/b.go", "unused import", 12]
    ]
  }
}
```



## Suggested implementation hook

Add a dispatcher in a new package (e.g. `internal/tools/compress`) and call it inside `internal/llm/tool_execution.go` after `tool.Execute` returns:

```go
output, err := tool.Execute(ctx, input)
if err == nil && output != nil {
    output = compressToolResult(tool.Name(), output)
}
```

`compressToolResult` dispatches by tool name, applies the relevant compressor, and only returns the compressed form if the estimated token saving exceeds the configured threshold. The original `output` is still stored in `HistoricalToolActivity.RawOutput`; the compressed version is used for `RetainedOutput` and for the provider-specific tool response payload.

## Next steps

1. Create `internal/tools/compress` package with a dispatcher and per-tool compressors.
2. Implement compression for `grep`, `glob`, and `bash` first (highest impact).
3. Wire `compressToolResult` into `internal/llm/tool_execution.go`.
4. Add unit tests for each compressor with before/after examples and size assertions.
5. Run `go test -race ./...`, `go mod tidy`, and `gofmt` as per project guidelines.
