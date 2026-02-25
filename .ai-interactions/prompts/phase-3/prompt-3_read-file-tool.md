## `ReadFile` Tool PRD

Now that we have a working tool integration, we will implement a `ReadFile` tool. Using this tool, LLMs will be able to read text files from the file system. We have the following strict requirements:

1. When the LLM invokes the `ReadFile` tool, Keen Code needs to ask for user's permission to read the file.
    - If the user grants permission, Keen Code should read the file and return the contents to the LLM.
    - If the user denies permission, Keen Code should not read the file and should return an error message to the LLM accordingly.
    - The permission has to be asked using the REPL interface. When permission is needed, REPL will render a prompt to the user asking for permission to read the file. Users can select "Allow" or "Deny" to grant or deny permission respectively using keyboard arrows and Enter key.
    - Carefully plan for the interaction flow and make sure the user experience is smooth and intuitive. And the implementation for the permission flow should be consistent with existing UI patterns in the codebase.
2. The `ReadFile` tool should only be able to read text files.
3. The `ReadFile` tool must respect the boundaries set in @internal/filesystem/guard.go
4. The `ReadFile` tool should return an error if the file is too large to read. For now, let's set the limit to 1MB.
5. The `ReadFile` tool should return an error if the file cannot be read for any reason. For example, if the file is binary, or if the file is not accessible, or if it's corrupted or not found at all.
6. The `ReadFile` tool should be able to read both relative and absolute paths.
7. The `ReadFile` tool should be able to read files from completely different directories.
8. All reads MUST follow the boundaries defined in @internal/filesystem/guard.go and respect user permissions.
9. For now, each time `ReadFile` is invoked, it should ask for permission again. However, we should have a future plan where we will add another permission level where users can select "Allow ReadFile for this session".

Based on the above requirements, create a design document for the `ReadFile` tool implementation. Then in the same document, add a granular todo list for the implementation. Save the document as in `.ai-interactions/phase-3/output-3_read-file-tool.md`.