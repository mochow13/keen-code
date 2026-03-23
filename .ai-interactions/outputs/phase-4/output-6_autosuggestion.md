# Slash Command Autosuggestion — Implementation Plan

## Overview

Add a prefix-based dropdown autosuggestion for `/` commands in the REPL input. When
the user types a `/` prefix, a dropdown appears below the textarea showing matching
commands (left: command name, right: description). Users navigate with arrow keys,
confirm with Enter or Tab, and dismiss with Esc.

---

## Architecture

### Layout (top → bottom in terminal)

```
┌──────────────────────────────────────┐
│  viewport (shrinks when dropdown     │
│  is visible)                         │
├──────────────────────────────────────┤
│  ┃ > /hel_                           │  ← textarea (moves up when dropdown shows)
├──────────────────────────────────────┤
│  /help   Show available commands     │  ← suggestion dropdown (below textarea)
│  /model  Change provider or model    │
│  /exit   Quit Keen                   │
├──────────────────────────────────────┤
│  Provider: openai   Model: gpt-4o    │  ← metadata
└──────────────────────────────────────┘
```

When the dropdown is visible:
- The dropdown renders between the textarea and the metadata row
- The viewport height is reduced by `dropdown.height()` to keep the total layout
  fitting the terminal height
- The textarea visually moves up because the viewport has shrunk, creating room for
  the dropdown below it

### Files to create / modify

| File | Action |
|------|--------|
| `internal/cli/repl/commands.go` | **Create** — command registry |
| `internal/cli/repl/suggestion.go` | **Create** — suggestion model & rendering |
| `internal/cli/repl/suggestion_test.go` | **Create** — unit tests |
| `internal/cli/repl/styles.go` | **Modify** — add dropdown styles |
| `internal/cli/repl/repl.go` | **Modify** — integrate suggestion into model, view, height |
| `internal/cli/repl/handlers.go` | **Modify** — intercept keys when dropdown is visible |

---

## Granular Todo Items

### 1. Create `commands.go` — command registry

- [ ] Define `slashCommand` struct:
  ```go
  type slashCommand struct {
      Name        string
      Description string
  }
  ```
- [ ] Declare `var allSlashCommands = []slashCommand` containing the three commands
  in alphabetical order:
  - `{"/exit", "Quit Keen"}`
  - `{"/help", "Show available commands"}`
  - `{"/model", "Change provider or model"}`
- [ ] Implement `filterCommands(input string) []slashCommand`:
  - Return empty slice when `input` is empty or does not start with `/`
  - Strip the leading `/` and compare case-insensitively as a prefix against each
    command name (without its `/`)
  - Return results sorted alphabetically by `Name` (already sorted in the source
    slice, so natural iteration order suffices)

### 2. Create `suggestion.go` — suggestion model

- [ ] Define `suggestionModel` struct:
  ```go
  type suggestionModel struct {
      visible  bool
      items    []slashCommand
      selected int  // index into items; -1 means no explicit selection
  }
  ```
- [ ] Implement `newSuggestionModel() suggestionModel` returning zero-value
  (invisible, empty)
- [ ] Implement `(s *suggestionModel) refresh(input string)`:
  - Call `filterCommands(input)` and store results in `s.items`
  - Set `s.visible = len(s.items) > 0`
  - Reset `s.selected = 0` whenever the visible list changes (so the first item is
    always pre-highlighted when the dropdown opens)
  - When no matches, set `s.visible = false` and clear `s.items`
- [ ] Implement `(s *suggestionModel) moveDown()` and `(s *suggestionModel) moveUp()`:
  - Clamp `selected` within `[0, len(items)-1]`; do not wrap
- [ ] Implement `(s suggestionModel) current() *slashCommand`:
  - Return `nil` when not visible or items is empty
  - Return `&s.items[s.selected]`
- [ ] Implement `(s suggestionModel) height() int`:
  - Return `0` when not visible
  - Return `len(s.items) + 2` (one line per item plus top and bottom border lines)
- [ ] Implement `(s suggestionModel) view(width int) string`:
  - Return `""` when not visible
  - Compute `cmdColWidth` as the length of the longest command name + 2 padding
  - For each item, render:
    - Left cell: command name, styled with `suggestionCmdStyle` (selected item uses
      `suggestionSelectedCmdStyle`)
    - Right cell: description, styled with `suggestionDescStyle` (selected item uses
      `suggestionSelectedDescStyle`)
    - Combine cells with `lipgloss.JoinHorizontal`
  - Wrap all rows in a bordered box using `suggestionContainerStyle` (rounded border,
    `primaryColor` foreground when an item is selected, `mutedColor` otherwise)
  - Pad or truncate the combined box to `width`

### 3. Add dropdown styles to `styles.go`

- [ ] Add `suggestionContainerStyle` — `RoundedBorder()`, border foreground
  `mutedColor`, no padding (border only)
- [ ] Add `suggestionCmdStyle` — foreground `secondaryColor`, fixed width equal to
  the longest command name
- [ ] Add `suggestionDescStyle` — foreground `mutedColor`
- [ ] Add `suggestionSelectedStyle` — background `primaryColor`, foreground white,
  bold; used as a wrapper around the full selected row
- [ ] Add `suggestionHintStyle` — foreground `mutedColor`, italic; used for the
  `↑↓ navigate  tab complete  esc dismiss` hint line rendered inside the border

### 4. Integrate into `replModel` (`repl.go`)

- [ ] Add `suggestion suggestionModel` field to `replModel`
- [ ] In `initialModel()`, set `model.suggestion = newSuggestionModel()`
- [ ] Update `adjustTextareaHeight()` to subtract `m.suggestion.height()`:
  ```go
  m.viewport.SetHeight(m.height - m.textarea.Height() - 4 -
      m.spinnerHeight() - m.suggestion.height())
  ```
- [ ] Update `applyWindowSize()` with the same subtraction (mirrors
  `adjustTextareaHeight`)
- [ ] Update `View()` to render the dropdown between the textarea and the metadata row:
  ```
  viewport output
  \n
  [spinner row — only when active]
  inputBorderStyle.Render(textarea)
  \n
  [suggestion.view(m.width) — only when visible]
  inputMetaView()
  ```

### 5. Update key handling (`handlers.go`)

Add a new `keyTab = "tab"` constant alongside the existing key constants.

In `handleKeyMsg()`, insert a new block **before** the existing `switch keyMsg.String()`
to intercept keys when the dropdown is visible:

- [ ] **Tab key** (dropdown visible, items present):
  - Replace textarea content with `s.current().Name` (or `items[0].Name` if no
    explicit selection)
  - Call `m.suggestion.refresh(m.textarea.Value())` — this will keep the dropdown
    open showing the full match if it still matches, or close it
  - Return without passing the key to textarea
- [ ] **Tab key** (dropdown not visible or no items): pass through to textarea as
  normal (insert a tab character)
- [ ] **Up arrow** (dropdown visible):
  - Call `m.suggestion.moveUp()`
  - Return without passing the key to textarea (prevents cursor moving up in input)
- [ ] **Down arrow** (dropdown visible):
  - Call `m.suggestion.moveDown()`
  - Return without passing the key to textarea
- [ ] **Enter** (dropdown visible):
  - Call `m.handleEnterKey()` — the current textarea value is already the typed
    prefix; selecting via Enter submits it (matches existing `/help` etc. handling).
    No special action needed: the dropdown will be closed automatically when textarea
    is reset after command execution.
  - _Alternatively_ (if we want Enter to autocomplete rather than execute): replace
    the textarea content with `s.current().Name` and close dropdown; discuss with
    team and decide before implementing.
  - **Decision for now**: Enter executes the command as-is (existing behavior); the
    dropdown is incidental.
- [ ] **Esc** (dropdown visible, no active stream): close the dropdown by calling
  `m.suggestion.refresh("")`; do not pass Esc to the stream interrupt handler
- [ ] After every key that is passed through to textarea (the default fall-through
  branch at the bottom of `handleKeyMsg`), call `m.suggestion.refresh(m.textarea.Value())`
  so the dropdown updates on every keystroke

### 6. Create `suggestion_test.go` — unit tests

- [ ] Test `filterCommands("")` returns empty slice
- [ ] Test `filterCommands("/")` returns all three commands in alphabetical order
- [ ] Test `filterCommands("/h")` returns `/help` only
- [ ] Test `filterCommands("/m")` returns `/model` only
- [ ] Test `filterCommands("/e")` returns `/exit` only
- [ ] Test `filterCommands("/xyz")` returns empty slice
- [ ] Test `filterCommands("/EXIT")` (uppercase) returns `/exit` (case-insensitive)
- [ ] Test `filterCommands("/help")` returns exactly `/help` (exact match)
- [ ] Test `suggestionModel.moveDown()` increments selected and clamps at max
- [ ] Test `suggestionModel.moveUp()` decrements selected and clamps at 0
- [ ] Test `suggestionModel.current()` returns `nil` when not visible
- [ ] Test `suggestionModel.height()` returns 0 when not visible, `len(items)+2`
  when visible
- [ ] Test `suggestionModel.refresh("/")` sets `visible = true` and populates items
- [ ] Test `suggestionModel.refresh("")` sets `visible = false`

### 7. Final checks

- [ ] Run `go test ./...` — all tests pass
- [ ] Run `go mod tidy` — no stray dependencies
- [ ] Manual smoke test:
  - Type `/` → dropdown shows all three commands
  - Type `/h` → only `/help` remains
  - Press Down → `/help` remains highlighted (only one item)
  - Press Tab → textarea fills with `/help`
  - Press Esc → dropdown closes, textarea retains value
  - Press Enter → `/help` executes normally
  - Type `/xyz` → dropdown disappears
  - Tab on `/xyz` → nothing happens

---

## Key Decisions & Constraints

| Topic | Decision |
|-------|----------|
| Dropdown position | Below textarea (between input border and metadata row), in the flat rendered string |
| Viewport height adjustment | Shrink by `suggestion.height()` while dropdown is visible |
| Enter behavior | Executes the current textarea value (no change from existing); does **not** autocomplete |
| Tab behavior | Autocompletes with `selected` item (defaults to index 0 = first alphabetical match) |
| Tab with no match | No-op (key ignored) |
| Esc with active stream | Stream interrupt takes priority; dropdown closes as a side effect of textarea reset |
| Esc with no stream | Closes dropdown only |
| Wrapping navigation | No wrap; clamp at boundaries |
| Max dropdown items | Show all matching items (max 3 given current command count) |
