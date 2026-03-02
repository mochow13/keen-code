## `GrepTool`PRD

As a next step, we want to build the `GrepTool` for LLMs to search for text in files. Following are the strict requirements:

1. `GrepTool` must use the same permission mechanism in the codebase and used in `ReadFile` and `Glob` tools.
2. `GrepTool` must be able to recursively search for the given pattern in the files in the directory tree starting from the working directory.
3. As input, `GrepTool` must accept a `pattern` parameter and a `path` parameter. The `path` parameter is optional and defaults to the working directory. There will also be an optional `include` parameter to specify a `glob` pattern to include files in the search. If it's empty or not provided, all text files in the current directory that are not blocked by the @guard.go and @gitawareness.go are included in the search.
4. `GrepTool` will have an `output_mode` parameter that can be one of `file` and `content`. If `file` is selected, the tool will return a list of file paths that match the pattern. If `content` is selected, the tool will return a list of lines that match the pattern along with the file name and line number.
5. If there is an error, `GrepTool` must return an error message and the error will be sent back to the LLM. Then the LLM can take proper actions.

Based on these requirements, create a design doc `output-6_grep-tool.md` and save it in @.ai-interactions/outputs/phase-3. In the design, don't include the complete impelementation. Rather, focus on the design and then include a granular todo list for the implementation.