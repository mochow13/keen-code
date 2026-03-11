## `edit_file` tool

### PRD

1. `edit_file` tool will be used to edit a file.
2. The file must exist. If it doesn't exist, `edit_file` tool will return an error.
3. The `edit_file` tool will take 4 inputs: `path`, `oldString`, `newString`, `shouldReplaceAll`.
    - `path` is the absolute or relative path to the file to edit.
    - `oldString` is the string to replace.
    - `newString` is the string to replace with.
    - `shouldReplaceAll` is a boolean that indicates whether to replace all occurrences of `oldString` with `newString`.
4. The tool will return `success` flag, `path`, and `replacementCount`.
5. The `edit_file` tool must use the existing permission mechanisms that are already used in other tools.
6. The UI for `edit_file` tool will be different than other tools. It will show the file content with the changes highlighted like a `git diff`. First the diff will be shown, then the user will be asked to confirm the changes. If the user confirms, the changes will be applied to the file.
7. If the permission is already granted for writing, the diff will be shown and also the file will be edited.
8. It's important that the UI experience is seamless for the user.

### Execution

1. Check the PRD in @.ai-interactions/prompts/phase-3/prompt-8_edit-file-tool.md and plan in @.ai-interactions/outputs/phase-3/output-10_edit-file-tool.md. Review them and let's check for potential overkill or premature optimisation.
2. We will ignore the possible improvements in the plan for now. Let's implement it.
3. If permission is already given, any edit the LLM is asked for doesn't happen. Also no diff is shown. Check why.
4. So what we want is this:
- If LLM tries to update a file but there is no permission, a permission is asked along with showing the diff.
- If permission is already given for the session, the diff is still shown.
Give me some possible approaches for this.
5. 