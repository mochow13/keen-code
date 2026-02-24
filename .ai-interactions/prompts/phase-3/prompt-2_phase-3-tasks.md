## Tool Design Implementation

1. Ok Kimi, time to start working. Create a todo list first. Then implement one by one.
2. Move the tool styles into @internal/cli/repl/styles.go.
3. In @internal/llm/genkit.go, there are two unused functions: extractTextContent and fromGenkitRole. They are not needed. Remove them.
4. Add a unit test for StreamChat in @internal/llm/genkit.go for successful tool invokation.