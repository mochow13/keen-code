## `write_file` Tool PRD

1. The `write_file` tool will enable LLMs to write files to the filesystem.
2. The `write_file` tool must use the existing permission mechanisms that are already used in other tools.
3. The `write_file` tool will take 2 inputs: `path`, `content`.
    - `path` is the absolute path to the file to write.
    - `content` is the content to write to the file.
4. If the file already exists, it will be overwritten.