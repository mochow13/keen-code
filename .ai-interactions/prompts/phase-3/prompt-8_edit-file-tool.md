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

1. Check the PRD in @.ai-interactions/prompts/phase-3/prompt-8_edit-file-tool.md and plan in @.ai-interactions/outputs/phase-3/output-10_edit-file-tool.md. Review them and let's check for potential overkill or premature optimisation.
2. We will ignore the possible improvements in the plan for now. Let's implement it.
3. If permission is already given, any edit the LLM is asked for doesn't happen. Also no diff is shown. Check why.
4. So what we want is this:
- If LLM tries to update a file but there is no permission, a permission is asked along with showing the diff.
- If permission is already given for the session, the diff is still shown.
Give me some possible approaches for this.
5. 