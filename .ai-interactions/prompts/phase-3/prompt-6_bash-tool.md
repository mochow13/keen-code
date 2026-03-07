## `bash` Tool PRD

The next tool we want to build for Keen Code is `bash`. It should allow LLMs to execute bash commands in the terminal. We have the following requirements:
1. The `bash` tool will enable LLMs to execute any bash commands that can be executed in a bash shell.
2. The `bash` tool must use the existing permission mechanisms that are already used in other tools, with an additional safebuard described in point 4.
3. The `bash` tool will take 3 inputs: `command`, `isDangerous`, `summary`.
4. `isDangerous` is a boolean that indicates whether the command is dangerous. If `isDangerous` is true, the user will be always asked for permission before executing the command, even if the permission is already granted by selection "Allow for this session". If `isDangerous` is false, only the existing permission mechanism will apply where the user can grant permission for a single command or for all commands.
5. `summary` will be a 5-10 words summary of the command showing what the LLM intends to do.
6. UI for `bash` tool will be different from other tools. It will have two parts: the first part will show the command being executed and the summary. The second part will stream the output of the command in real-time. Both input and output must be rendered in markdown code blocks for bash.