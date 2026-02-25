## Glob Tool PRD

We need to implement a `glob` tool that allows LLMs to search for files using glob patterns. It needs to follow the following strict requirements:

1. When the LLM invokes the `glob` tool, Keen Code needs to ask for user's permission to search for files. Use the existing permission mechanism already implemented in the codebase and used for `read_file` tool. Reuse the existing code.
2. The `glob` tool should only be able to search for files based on a pattern.
3. The `glob` tool must respect the boundaries set in @internal/filesystem/guard.go. It shouldn't be able to list files that are not allowed.
4. The `glob` tool should return an error if the search is too large to search. For now, let's set the limit to 1000 files.
5. The `glob` tool should return an error if the search cannot be performed for any reason. For example, if the search pattern is invalid, or if the search is not accessible, or if it's corrupted or not found at all. Such error should also be sent to the LLM so that LLM can take proper actions.
6. The `glob` tool should be able to search both relative and absolute paths.
7. The `glob` tool should be able to search files from completely different directories. But of course, it should still respect the boundaries defined in @internal/filesystem/guard.go and user permission.
