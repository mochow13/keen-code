## `edit_file` tool

### PRD

We want to now create an `edit_file` tool that will enable LLMs to edit files.

1. `edit_file` tool will be used to edit a file. It can edit any UTF-8 files just like how it can read them in `read_file` tool. LLM must read a file at least once before editing it.
2. The file must exist. If it doesn't exist, `edit_file` tool will return an error.
3. The `edit_file` tool will take 4 inputs: `path`, `oldString`, `newString`, `shouldReplaceAll`.
    - `path` is the absolute or relative path to the file to edit.
    - `oldString` is the string to replace.
    - `newString` is the string to replace with.
    - `shouldReplaceAll` is a boolean that indicates whether to replace all occurrences of `oldString` with `newString`.
4. The tool will return `success` flag, `path`, and `replacementCount`.
5. The `edit_file` tool must use the existing permission mechanisms that are already used in other tools.
6. The UI for `edit_file` will be very specific and different from other tools:
    - When LLM wants to use `edit_file` tool, the REPL will first show the tool call like how it's done for `write_file` tool.
    - After that, REPL will show the changes being made like a diff in Github pull request. To achieve this, we need to make sure the UI is neat and colored properly to provide a seamless experience to the users.
    - For now, we will show the full diff in a card. We will not implement collapse feature.
    - The diff also needs to show line numbers on the left from the file.
    - Finally, the existing permission card will be rendered if user has not already given session permission for the `edit_file` tool. If user gives permission, then only the file will be edited. If user denies, then an error will be returned and passed to LLM.
    - If session permission is granted for writing, only diff will be shown, and the file will be edited directly.
7. It's important that the UI experience is seamless for the user.

### Execution

1. First, we have got rid of operations.go. So we don't need to have EditOperation anymore in the plan.
2. About showing diff, is there anything we can reuse, like a library or so? Manually implementing the diff is complicated.
3. We are managing editLines through permissionRequester. Why not treat it as a segment?
4. We are still implementing the interface by `REPLPermissionRequester`. Can we have a completely decoupled approach?
5. The types and interface outlined in Step 1 should not be in permission.go. Let's put it in its own file.
6. Review the entire plan and make sure diff generation and UI rendering for diff are decoupled from permission mechanism.
7. Ok let's implement the plan.