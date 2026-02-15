# LLM Integration - `StreamChat`

As a first iteration, we will implement `StreamChat` feature in the REPL. To begin, first take a look at the RFC in @.ai-interactions/outputs/phase-1/output-1_rfc.md. The RFC provides a high-level overview of the LLM integration as a diagram in the `Phase 2` section.

## Simple `StreamChat` Implementation

Users will write their messages in the REPL. A message can have a new line but users have to press `ctrl+enter` for a new line. A message is sent to the LLM when the user presses `enter`. The LLM's response will be streamed in the REPL. The response should be formatted nicely using lipgloss.

As discussed in the RFC, we will use genkit for LLM integration. Use `mcp-deepwiki` to understand how to use genkit for LLM integration.

In our project directory, we will create a new package `internal/llm` and put all the LLM integration code there. The package will have a Go interface `LLMClient` with a method `StreamChat`. For now, this is the only method we will implement.

For chatting, we need to pass current config from REPL to LLM client. Based on the provider, model, and API key, we will create the LLM client accordingly. Note that users can change the provider and model in the REPL using `/model` command. So, we need to also update the LLM client mid-conversation.

If there is an error while generating response, we should print the error message in the REPL and the REPL should continue looping. For now, if there is an error, we don't need to retry.

Based on the requirements above, generate a list of small to-do items to implement this feature. Save them as a markdown file in @.ai-interactions/outputs/phase-2/output-3_stream-chat-todo.md.