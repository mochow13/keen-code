## Tool Design Implementation

1. Ok Kimi, time to start working. Create a todo list first. Then implement one by one.
2. Move the tool styles into @internal/cli/repl/styles.go.
3. In @internal/llm/genkit.go, there are two unused functions: extractTextContent and fromGenkitRole. They are not needed. Remove them.
4. Add a unit test for StreamChat in @internal/llm/genkit.go for successful tool invokation.
5. Read and create a plan for the prompt in @.ai-interactions/prompts/phase-3/prompt-3_read-file-tool.md.
6. There is a read_file tool design in @.ai-interactions/outputs/phase-3/output-3_read-file-tool.md. And there is a also a granular implementation todo list. Let's get to work.
7. I manually tested and get this error: "I am sorry, I cannot fulfill this request. The  read_file  function only supports text files. The .go extension indicates that the file is a Go source file, which is not a plain text file."
8. Should we rather siimplify the check and directly read any file, and only fail if the file contains something else except characters?
9. Check the plan in @.ai-interactions/outputs/phase-3/output-3_read-file-tool.md and check the code and ensure that everything is implemented correctly.
10. If user denies permission for reading a file, we will take the error and send it to the LLM so it can handle it gracefully. Let's implement this.
11. We have a spinner that is supposed to animate in the REPL. But right now it's not animating. Let's fix this.
12. In the read_file tool design, we proposed to use MIME type detection. But we have decided to discard this approach. Let's update the design doc by removing MIME type detection.
13. How can we refactor the Update() function in @internal/cli/repl/repl.go? Let's break it down into smaller, deduplicated functions.