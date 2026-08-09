# Plan: Hashline-Style Edit for Keen (Issue #80)

## Executive Summary

Replace `edit_file`'s exact `oldString`/`newString` mechanism with line-numbered hash anchors and multi-operation edits. The design is informed by hashline-style tools, including [pi-hashline-edit](https://pi.dev/packages/pi-hashline-edit), but is intentionally smaller and tailored to Keen.

**Locked decisions:**

1. **Modify `read_file` and `edit_file` in place.** `edit_file` changes to accept an `ops[]` array of anchored edits. `write_file` is unchanged.
2. **Anchor format is `N:HASH`.** `N` is the line number and `HASH` is a three-hex-character hash of that line's raw bytes.
3. **Use FNV-1a from Go's standard library.** A 32-bit FNV-1a digest is computed for each line; three hex characters are displayed. This is a fast, non-cryptographic typo and local-content check, not a security boundary.
4. **No file hash.** There is no whole-file digest in `read_file` and no global optimistic-lock parameter in `edit_file`. An unrelated change elsewhere in a file must not reject an otherwise valid anchored edit.
5. **Multi-op calls use one snapshot.** Every op in one `edit_file` call is validated against the same pre-edit file state, then all ops are applied bottom-up in memory and written atomically.
6. **No context hashing or relocation.** A hash covers only its line. A line/hash mismatch fails loudly; the tool never searches for a “close enough” target.
7. **`grep` gets line hashes as a fast follow.** Structured content matches gain `line_hash`, so their `line_number` and `line_hash` directly form an edit anchor.
8. **Benchmarks gate adoption.** Compare exact-match and hashline behavior across models before declaring the format final.

## Problem Statement

Keen's current `edit_file` tool uses exact string matching (`oldString`/`newString`). This approach:

1. **Duplicates old content in edit requests.** The model must transmit the complete text it intends to replace as well as its replacement.
2. **Fails on transcription errors.** A small mismatch in whitespace, indentation, or surrounding context causes a “not found” error.
3. **Can target the wrong repeated content.** By default, a repeated `oldString` changes its first occurrence; models need enough surrounding text to select the intended occurrence, while `shouldReplaceAll` changes every occurrence.
4. **Has no compact positional intent check.** A model cannot cheaply state both “line 42” and “the content I expect at line 42,” which is useful for rejecting off-by-one and miscoded-line errors before a write.

### Current Implementation (as of this writing)

- `read_file` returns formatted content in a structured result. That content prefixes lines with `N: ` (`internal/tools/read_file.go:236`), truncates long lines at 1000 runes (`read_file.go:232-235`), and paginates through `offset`/`limit`.
- `edit_file` verifies that `oldString` occurs at least once (`internal/tools/edit_file.go:135-137`), replaces either the first occurrence or all occurrences according to `shouldReplaceAll` (`edit_file.go:141-146`), and does not reject a non-`shouldReplaceAll` edit merely because the string occurs more than once.
- For memory files, the proposed final content is scanned for secrets before permission is requested and before any write (`memory.ContainsSecret`, `edit_file.go:149-151`). Path-policy checks run before the read; the user permission prompt, when required, occurs after the diff preview.
- `edit_file` computes a unified diff from the full original and final content (`computeEditDiff`, `edit_file.go:183-227`) and sends it to the CLI `DiffEmitter` before permission is requested and before the write. It is a user-facing preview, not part of the tool result returned to the model.
- File writes are not atomic: `os.WriteFile` writes in place (`edit_file.go:168`).
- Tool calls execute strictly sequentially in `internal/llm/genkit.go`'s `executeTools` loop. There are no goroutines or parallel execution in the tool pipeline. Multiple calls emitted in a model turn run one after another.

## Background: Hashline Editing

Hashline output attaches a compact anchor to every displayed line:

```text
1:a3f|package tools
2:9c1|
3:e47|import "fmt"
```

The model references `LINE:HASH` and supplies only the replacement text. The line number identifies the intended location; the hash verifies that the current line is the one the model saw. This catches model-side errors such as copying `42` as `24` or editing an adjacent line by mistake.

There is deliberately **no whole-file validation**. A change far from an anchored line does not invalidate the local edit. If a change alters or shifts the anchored target, the line/hash validation rejects the op and asks the model to re-read.

### Expected Benefits (from published sources)

| Metric | Exact-Match | Hashline | Claimed Improvement |
|---|---:|---:|---:|
| First-attempt edit success | ~68% | ~99.5% | +31.5 points |
| Tokens per edit | ~450 | ~195 | 2.3x fewer |
| Retry loop depth | 3–5 attempts | 1 attempt | Fewer round trips |

These figures come from tool authors, not independent benchmarks. Prior work also shows weaker models can regress on hashline syntax. Keen must measure its own default and supported models before treating these claims as product facts.

## Design Decisions

### 1. Evolve Existing Tools

Modify existing tools rather than introducing a parallel editing interface:

- **`read_file`:** prefix each displayed line with `N:HASH|`.
- **`edit_file`:** replace `oldString`, `newString`, and `shouldReplaceAll` with an `ops[]` array of anchored edits.
- **`grep` (fast follow):** add `line_hash` to structured content matches.

This keeps existing permission checks, secret scanning, user-facing diff preview, and file-change reporting inside `edit_file`.

### 2. Hash Function: FNV-1a, Three Hex Characters

Use `hash/fnv` from the Go standard library:

```go
// Three displayed hex characters from FNV-1a over raw line bytes.
func computeLineHash(line []byte) string {
    h := fnv.New32a()
    _, _ = h.Write(line)
    return fmt.Sprintf("%08x", h.Sum32())[:3]
}
```

Properties:

- **Fast:** one simple byte-wise pass; the tool hashes each displayed/matched line.
- **Stable:** deterministic across processes and sessions, unlike `hash/maphash`.
- **No dependencies:** standard library only.
- **Non-cryptographic by design:** the hash is an accidental-error detector, not protection against a hostile collision.

Three hex characters provide 12 bits. Because the line number is also mandatory, a collision matters only when the line at the stated position changes to another value with the same displayed hash. That is acceptable for this compact local guard and will be covered by tests and benchmarks.

### 3. No File Hash and No Context Hashing

Do not calculate, display, accept, or validate a file-level hash.

A strict whole-file hash would reject an edit after any unrelated modification anywhere in the file. That would narrow an existing capability: today an exact-match edit can still succeed if another edit changes unrelated text. Hashline should retain this property where local anchors remain valid.

Do not adopt pi-hashline-edit's neighbor/context hashing either:

- `N:HASH` already disambiguates identical content such as blank lines, `}`, and `return nil`; the line number supplies positional identity.
- Context hashes would invalidate neighboring anchors after every edit and add a less obvious model interaction.
- Keen's v1 needs a simple local rule: the target line's own bytes must hash to the supplied value at validation time.

### 4. Multi-Op Calls, One Snapshot, No Relocation

All edits to a single file that are known at once belong in one call:

- Read the file and obtain `N:HASH` anchors.
- Submit one `edit_file` call with `ops: [...]`.
- Validate every anchor against the same pre-edit content.
- Reject the entire call if any anchor is malformed, out of range, mismatched, or overlaps another op.
- Apply validated operations in descending line order, preserving the addresses of lower operations.
- Create one final content value, preview its full unified diff, request permission if needed, then atomically write it.

This gives multi-spot same-file edits all-or-nothing behavior. Operations do not invalidate one another because validation happens before mutation and application is based on one snapshot.

Separate calls for different files may be emitted in one model turn. Keen currently runs them sequentially, but separate paths do not affect one another.

A later edit to the same file must call `read_file` again for now. Chaining new anchors or state from an edit response is out of scope.

**No relocation:** if `42:a3f` does not match the current line 42, reject it. Do not search the file for the hash, use a fuzzy match, or apply to a nearby line. The error must name the failing op and suggest a bounded re-read window.

### 5. Diff Preview and Edit Result

Keep `computeEditDiff` and the CLI `DiffEmitter` behavior unchanged in purpose and format. It compares complete old and final file contents, so it already correctly represents multiple hashline operations, separate hunks, and line-number shifts.

The preview remains a plain user-facing unified diff. Do not add hashes to it: the model does not receive the emitter output, and hashes would add visual noise for the user.

The preview occurs after all proposed ops have been validated and final content has been assembled, but before permission and the write, as it does today. A successful result retains the existing success/path/change summary and does not return new anchors.

### 6. `grep` Adds Line Hashes (Fast Follow)

`grep` currently returns structured content matches with `file`, `line_number`, and `line` fields (`internal/tools/grep.go:382-386`). Add `line_hash` to each content match. A search result then supplies a usable local edit anchor:

```json
{
  "file": "internal/tools/example.go",
  "line_number": 42,
  "line_hash": "a3f",
  "line": "return nil"
}
```

The model can use `42:a3f` in an edit op. Because there is no file hash, a grep result does not need an intermediate `read_file` solely to obtain global state. A model should still read surrounding code when it needs context or when an anchor fails.

Reuse the same FNV-1a helper. `grep` has structured results rather than colorized/context output, so no display-format compatibility work is required.

### 7. Tool Usage Guidance

Add this instruction to the `edit_file` description and reinforce it once in the system prompt:

> Put all edits to one file in one `ops` array. The tool validates every anchor against one file snapshot and applies the edits atomically. You may emit separate calls for different files in one turn. Read a file again before making a later same-file edit.

Errors reinforce the same pattern. No executor concurrency changes are needed.

### 8. Explicitly Out of Scope

- **File hash / whole-file optimistic lock:** rejects unrelated changes and is intentionally omitted.
- **Context-based hashing:** unnecessary for `N:HASH` v1 and adds neighbor-invalidation semantics.
- **Hash-only anchors:** duplicate content would be ambiguous and models lose line-number spatial context.
- **Anchor relocation / three-way merge:** never silently alter the requested target.
- **`replace_text` exact-match fallback:** reintroduces the failure modes being replaced; revisit only if benchmarks show material weak-model regressions.
- **No-op loop guard and strict patch-content validation:** useful future guards if benchmarks expose the failure modes.
- **Parallel tool execution:** an independent future improvement; sequential execution is correct today.

## Technical Design

### `read_file` Changes

Output format for all files:

```text
read_file operation completed

File: internal/tools/example.go (228 bytes)

1:a3f|package tools
2:9c1|
3:e47|import "fmt"
4:9c1|
5:2b8|func Greet(name string) string {
6:7f0|	return fmt.Sprintf("Hello, %s", name)
7:c5e|}
```

Rules:

- Hash raw line bytes, excluding the line-ending delimiter and line number.
- Compute the hash before display truncation, so it corresponds to the full current line that `edit_file` validates.
- Do not append a file-hash footer.
- The output-format section of `internal/llm/systemprompt.go` documents `N:HASH|content`.

### `edit_file` Schema

```json
{
  "path": "internal/tools/example.go",
  "ops": [
    {
      "start": "6:7f0",
      "text": "\treturn fmt.Sprintf(\"Hello, %s!\", name)"
    },
    {
      "op": "insert_after",
      "start": "7:c5e",
      "text": "\n// done"
    },
    {
      "start": "1:a3f",
      "end": "3:e47",
      "text": "package main"
    }
  ]
}
```

| Field | Type | Description |
|---|---|---|
| `path` | string | File path (unchanged) |
| `ops` | array | Non-empty array of edit operations |
| `ops[].op` | string | `replace` (default), `insert_after`, `insert_before`, `insert_head`, or `insert_tail` |
| `ops[].start` | string | Required anchor, `LINE:HASH`, except for head/tail insertion |
| `ops[].end` | string | Optional inclusive end anchor for a range replacement |
| `ops[].text` | string | Replacement/inserted text; may be multiline; empty text deletes a `replace` range |

Rules:

- `insert_head` and `insert_tail` need no anchor. `insert_head` is the only valid operation for an empty file.
- Explicit insert operations avoid ambiguous sentinel anchors such as `0:000`.
- `start` and `end` are order-independent; normalize them if reversed.
- Overlapping ranges or contradictory insertions at the same anchor are errors.
- Validate both the line number and the line hash for every supplied anchor before assembling final content.

### Errors

```text
Error: op 2: line hash mismatch at 42:7f0 (actual a1b). The line content
has changed or the anchor is incorrect. Re-read lines 32-52 and retry with
fresh anchors. To edit several parts of one file, pass all edits as ops in
a single call.

Error: op 1: line number 150 is out of range (file has 100 lines).
Re-read the file to obtain current anchors.

Error: ops 1 and 3 have overlapping ranges (lines 5-8 and 7-10).
```

### Atomic Writes

Ship atomic writing as a standalone prerequisite change, benefiting both `edit_file` and `write_file`:

1. Write the final bytes to a temporary file in the target directory.
2. `fsync` and close the temporary file.
3. Preserve target permissions.
4. Rename over the target on POSIX.
5. Resolve symlink targets so the referent is updated while the symlink remains; clean up temporary files on errors.

This prevents readers from observing partial writes and makes the validated multi-op update all-or-nothing at the file-content level.

## Implementation Plan

### Phase 0: Atomic Writes (Standalone PR)

- Extract a temp-file-plus-rename helper in `internal/tools`.
- Use it from `edit_file` and `write_file`.
- Test permission preservation, symlink behavior, and temporary-file cleanup on failure.

### Phase 1: Core Hashline Plumbing

- Add `internal/tools/hashline.go` with `computeLineHash`, anchor parsing/validation, and raw-line splitting helpers.
- Preserve raw line-ending behavior needed by both display and edits.
- Add focused unit tests for hashes, parsing, and line splitting.

### Phase 2: `read_file` and `edit_file` Integration

- Make `read_file` emit `N:HASH|` line prefixes.
- Replace the `edit_file` schema with `ops[]` and implement one-snapshot validation, overlap detection, bottom-up application, and atomic writing.
- Preserve the existing full-file unified CLI diff preview and existing success/path/change result shape.
- Update the tool description and system prompt with the multi-op guidance.
- Add integration coverage for the full edit matrix below.

### Phase 3: `grep` Hashes

- Add `line_hash` to structured content matches using the Phase 1 helper.
- Test structured results; retain existing binary-file and match-limit behavior.

### Phase 4: Benchmarks

- Build a scripted harness that replays the same scenarios against exact-match and hashline formats across several models.
- Measure first-attempt success, tokens consumed, and retries.
- Start with: single-line replacement, multi-line range replacement, insertion, deletion, multi-spot same-file edit, multi-file calls, changed-anchor retry, repeated-content targeting, and long/truncated lines.
- Test at least the default Gemini model, one Anthropic model, one OpenAI model, and a weaker open model if available.
- Hashline must not regress first-attempt success on the default model and must show a material token benefit before the format is considered final.

### Phase 5: Documentation and Polish

- Update README and tool documentation.
- Record a demo using the existing cast-to-GIF workflow from `AGENTS.md`.

## Tasks

Complete these tasks in order. Each task should be a small, reviewable change. Do not add file-level hashes, context hashes, anchor relocation, chaining state in edit results, or parallel tool execution.

### Task 0 — Reconfirm the Baseline

- Read `internal/tools/read_file.go`, `edit_file.go`, `write_file.go`, `grep.go`, `diff.go`, their tests, `internal/llm/systemprompt.go`, and tool registration.
- Run the existing test suite before modifying code.
- Record any implementation facts that differ from this plan and resolve the discrepancy before proceeding; do not silently change the locked design decisions.

**Done when:** the worker understands the current tool schemas, permission flow, diff-preview flow, and structured grep result shape.

### Task 1 — Add the Atomic Write Helper

- Add an internal `tools` helper that writes a complete byte slice through a temporary file in the target directory, syncs and closes it, preserves the existing target's permissions, and renames it over the target.
- Resolve a symlink target before writing so the symlink remains a symlink and its referent changes.
- Ensure temporary files are removed on every error path.
- Replace direct `os.WriteFile` use in both `edit_file` and `write_file` with this helper without changing either tool's public schema or normal result shape.

**Tests:** ordinary replacement; existing-mode preservation; symlink referent update; failure cleanup. Do not claim cross-platform atomicity beyond the platforms Keen supports.

**Done when:** current `edit_file` and `write_file` tests pass unchanged, plus new helper coverage passes.

### Task 2 — Add Isolated Hashline Primitives

- Create `internal/tools/hashline.go` and `hashline_test.go`.
- Implement a single FNV-1a 32-bit line-hash function which accepts raw line content bytes and exposes exactly the first three lowercase hexadecimal characters.
- Implement parsing and validation for the exact `LINE:HASH` anchor grammar: decimal line numbers are 1-based and positive; hashes are exactly three lowercase hexadecimal characters.
- Implement one raw-line splitter shared by future read formatting and edit validation. Define and test LF, CRLF, empty input, consecutive blank lines, and a trailing line ending.
- Decide whether CRLF's `\r` is excluded as part of the delimiter (recommended) and encode that decision consistently in tests and documentation.

**Tests:** fixed FNV vectors; repeatability; malformed anchors; out-of-range line numbers; LF/CRLF; empty and trailing-newline files.

**Done when:** no existing tool behavior changes and every new primitive is covered by focused tests.

### Task 3 — Render Hashline Anchors in `read_file`

- Replace the current `N: ` line prefix with `N:HASH|`, using the Task 2 helper.
- Hash the complete logical line before the existing 1000-rune display truncation; preserve the existing truncation marker and pagination behavior.
- Apply the same output format to blank lines and all text files.
- Do not add a file-hash footer or change `read_file` parameters/result fields beyond the formatted content.
- Update relevant `read_file` tests and the output-format documentation in `internal/llm/systemprompt.go`.

**Tests:** normal lines; duplicate text on different lines; blank lines; long truncated lines whose hash reflects full content; CRLF; pagination offsets; empty files.

**Done when:** every displayed addressable line has one stable `N:HASH|` prefix and existing non-format behavior remains unchanged.

### Task 4 — Define and Register the Multi-Op `edit_file` Schema

- Replace `oldString`, `newString`, and `shouldReplaceAll` in the `edit_file` tool schema with a required non-empty `ops` array.
- Support the locked operations: `replace` (default), `insert_after`, `insert_before`, `insert_head`, and `insert_tail`.
- Require `start` for anchored operations; permit optional inclusive `end` only for `replace`; require no anchor for `insert_head`/`insert_tail`.
- Keep `path` handling, permission behavior, secret checking, diff emission, and change-reporting integration intact.
- Update the tool description with the one-file/one-ops-array rule, and reinforce it once in `systemprompt.go`.

**Tests:** schema validation for missing/empty ops, unsupported operation names, required/misplaced anchors, and invalid range fields.

**Done when:** the model-facing schema and descriptions accurately teach that known same-file changes belong in one call.

### Task 5 — Implement Snapshot Validation and In-Memory Application

- Read the target exactly once for the edit operation and split it using Task 2's shared representation.
- Parse and validate every supplied anchor against that immutable pre-edit snapshot before constructing final content.
- For range replacements, normalize reversed valid start/end anchors; reject malformed, out-of-range, or hash-mismatched anchors.
- Reject overlapping replacement ranges and contradictory insertions at the same anchor. Specify and test whether an insertion at a replacement boundary is allowed; prefer rejecting ambiguous combinations.
- Transform all validated ops to a final content value in descending line order, preserving lower line addresses.
- Treat replacement with empty `text` as deletion. Permit only `insert_head` for an empty file.
- On any validation failure, make no permission request and write nothing. Error messages identify the failing op and include a bounded re-read suggestion.

**Tests:** single-line and multi-line replacement; insertion before/after/head/tail; deletion; empty file; reversed range; all-or-nothing validation; same-call multi-spot edits; overlap/contradiction errors; wrong line number; wrong hash; changed target; target shifted by insertion above it; unrelated change far from an unchanged target.

**Done when:** all operations validate against one snapshot, generate one final content value, and never partially apply.

### Task 6 — Integrate Preview, Permissions, Secrets, and Atomic Write

- Feed the complete original and final content to the existing `computeEditDiff`; do not build a per-op diff implementation.
- Keep the existing plain unified diff preview sent through `DiffEmitter` before a pending permission prompt and before writing. Do not add hashes to that UI preview.
- Run the existing memory-secret check against the final content before requesting permission or writing.
- Use the Task 1 atomic writer for the final content only after all validation, preview, secret checks, and permission handling succeed.
- Preserve the current successful tool-result shape (`success`, path, replacement/change reporting). Do not return fresh anchors or support chaining without another `read_file`.

**Tests:** a multi-op edit produces the expected existing unified diff; denied permission leaves content unchanged; secret rejection leaves content unchanged; successful edit reports the same fields expected by current consumers; atomic write path is used.

**Done when:** hashline changes tool addressing only; user review and safety behavior remain compatible.

### Task 7 — Add `line_hash` to Structured `grep` Matches

- Extend the content-match result type in `internal/tools/grep.go` with `line_hash`.
- Compute it with the shared Task 2 helper over the complete matched line, not a highlighted or truncated representation.
- Preserve all existing fields and match-limit/binary-file behavior.
- Document that `line_number` plus `line_hash` forms an edit anchor; models should still read nearby context when needed.

**Tests:** a normal structured content match has the expected three-character hash; repeated lines share the hash but retain distinct line numbers; binary and match-limit paths do not regress.

**Done when:** grep results provide directly reusable local anchors without changing their existing structured shape.

### Task 8 — Build the Benchmark Harness and Make the Decision

- Implement a repeatable harness outside production tool code that can run comparable edit scenarios using the legacy exact-match format and hashline format.
- Include: single-line replacement, multi-line range replacement, insertion, deletion, multi-spot same-file edit, multiple files, changed-anchor retry, repeated-content targeting, long/truncated lines, and an unrelated external modification.
- Measure first-attempt success, request/response token use where provider telemetry permits, and retry count.
- Run at least the default Gemini model, one Anthropic model, one OpenAI model, and one weaker open model if available.
- Record raw configurations, scenario fixtures, model versions, and results in a reviewable artifact. Do not present vendor/tool-author claims as Keen results.

**Done when:** the project has enough data to confirm that hashline does not reduce default-model first-attempt success and provides a material token benefit, or to revise/reject the rollout.

### Task 9 — Final Documentation and Validation

- Update README/tool documentation with the final `read_file` anchor format and `edit_file` multi-op examples.
- Explain the important limits: no file hash, no anchor relocation, one snapshot per call, and re-read before a later same-file edit.
- Add or update an interaction demo only after the schema is stable.
- Run `gofmt` on modified Go files, `go mod tidy`, and `go test -race ./...`.

**Done when:** documentation matches the implementation and the required formatting, dependency, and race-test checks pass.

## Critical Files

| File | Change |
|---|---|
| `internal/tools/read_file.go` | Emit `N:HASH|` line prefixes |
| `internal/tools/edit_file.go` | Schema swap to `ops[]`; snapshot validation; bottom-up application; preserve CLI diff preview |
| `internal/tools/grep.go` | Add `line_hash` to structured content matches (Phase 3) |
| `internal/tools/hashline.go` | New line-hash, anchor parsing, and line-splitting helpers |
| `internal/tools/write_file.go` | Use atomic-write helper (Phase 0) |
| `internal/llm/systemprompt.go` | Document anchor format and one-call-per-file guidance |
| `internal/tools/*_test.go` | Unit and integration coverage |

## Testing Strategy

### Unit Tests

- `hashline_test.go`: FNV-1a vectors, three-character formatting, valid/malformed anchors, range normalization, LF/CRLF splitting, trailing newline, and empty files.
- `read_file_test.go`: hash-prefix format, stability across calls, hash over a complete truncated line rather than its display prefix, blank lines, and empty files.
- `edit_file_test.go`: line-number and line-hash validation, all-or-nothing multi-op validation, bottom-up application, overlapping ranges, inserts, empty-file insert, deletion through empty replacement text, error messages, permission and secret checks, and atomic writes.
- `grep_test.go` (Phase 3): `line_hash` in structured content matches.

### Integration Tests

- Read → one multi-op same-file edit.
- Read → external change far from the anchored range → edit still applies if its anchors remain valid.
- Read → modify or insert above an anchored target → anchor mismatch rejects with a re-read hint.
- Repeated-content lines remain distinguishable through their distinct line numbers.
- Multiple calls for different files emitted in one turn all succeed under the existing sequential executor.

### Regression Risks

| Risk | Mitigation |
|---|---|
| Weaker models regress on anchor syntax | Benchmark gate; consider a fallback only if data justifies it |
| Read output has extra tokens | Three-character hashes add five visible characters per line; measure against edit-side savings |
| Three-character collision | Mandatory line number, FNV test vectors, and benchmark validation; length can be revisited with data |
| CRLF mismatch | One raw-line implementation shared by display and validation |
| Models copy `N:HASH|` into replacement text | Add strict patch validation later if benchmarks show it occurs |

## Edge Cases

1. **CRLF:** keep `\r` behavior consistent between line splitting, displayed line content, and hash validation. Hash the content bytes excluding the `\n` delimiter; decide and test whether a preceding `\r` is content or a delimiter once, then use that rule everywhere.
2. **Trailing newline:** define whether a terminal `\n` creates an addressable final empty logical line. The current `splitLines` (`read_file.go:242-251`) drops it; hashline display and edit semantics must agree.
3. **Long lines:** hash the full raw line, not the 1000-rune display truncation.
4. **Unicode:** hash raw UTF-8 bytes. NFC and NFD differ, correctly.
5. **Empty file:** no addressable lines; only `insert_head` is valid.
6. **Changed target:** if the current line changed, moved due to an insertion above it, or the stated line is out of range, reject the anchor rather than relocating it.
7. **Unrelated external change:** no file hash means an edit can proceed if every supplied local anchor still validates.
8. **Batched same-file calls:** encourage one multi-op call. If a preceding call shifts or changes a later call's target, the later call fails line validation and must re-read; if its target remains unchanged at the stated line, it may validly succeed.

## Effort Estimate

| Component | Effort |
|---|---:|
| Phase 0: Atomic writes | 0.5 day |
| Phase 1: Hashline plumbing | 1 day |
| Phase 2: read/edit integration and prompts | 2–3 days |
| Phase 3: grep hashes | 0.5 day |
| Phase 4: Benchmark harness and runs | 2–3 days |
| Phase 5: Documentation and demo | 0.5 day |
| **Total** | **~7–9 days** |

## Success Criteria

1. `read_file` emits stable `N:HASH|` anchors; `grep` includes `line_hash` in structured content matches in Phase 3.
2. `edit_file` applies validated multi-op edits atomically, without a whole-file hash gate.
3. Unrelated changes elsewhere in a file do not reject an edit whose local anchors remain valid.
4. Existing permission, secret-check, CLI diff-preview, and change-reporting behavior has no regressions.
5. Phase 4 shows a material token reduction and no first-attempt regression on the default model.
6. Errors consistently guide the model to re-read and retry with fresh local anchors.
