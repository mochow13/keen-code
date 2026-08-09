## Hashline Implementation

1. Remove all the comments in `internal/tools/edit_file.go`.
2. Some functions in `internal/tools/edit_file.go` should live in `internal/tools/hashline.go`. Which ones should we move?
3. Anything handling anchors is hashline logic, right? Move `validateAnchor`, `reReadWindow`, and `joinLines` to `internal/tools/hashline.go`.
4. Remove the comments in `internal/tools/hashline.go`.
5. Simplify anchor validation: for each operation, check whether its anchors exist in the current snapshot, and ensure operations do not overlap. Treat anchors as opaque identifiers rather than validating the `N:HASH` structure.
6. Move operation parsing and validation functions to a new file, `internal/tools/hashline_validation.go`, without changing the validation logic.
7. Simplify `validatedEditOp`: remove `pos`, use `start` and `end` as zero-based half-open ranges, and sort edits by their resolved range since conflicts are already validated.
8. Remove `replacementCount` and `file_changed` from the `edit_file` output. Keep only the success status and the path of the changed file.
9. Consider preserving each line's original separator in `joinLines` instead of applying one separator to the whole file.
10. Fix overlap validation for half-open replacement ranges: insertions immediately before the first replaced line or immediately after the last replaced line should not conflict. An insertion overlaps only when `replacement.start < insertionPosition && insertionPosition < replacement.end`; define shared-boundary ordering separately.
