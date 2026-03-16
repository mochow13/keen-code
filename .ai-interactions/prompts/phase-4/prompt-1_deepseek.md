## Supporting `deepseek-reasoner`

1. Currently, `deepseek-chat` works as expected but `deepseek-reasoner` doesn't. It fails with this error:
```
Error: stream error: POST "https://api.deepseek.com/chat/completions": 400 Bad Request {"message":"Missing
`reasoning_content` field in the assistant message at message index 1. For more information, please refer to
https://api-docs.deepseek.com/guides/thinking_mode#tool-
calls","type":"invalid_request_error","param":null,"code":"invalid_request_error"}
```
Let's figure it out why.
2. I think we need to provide `reasoning_content` field for `deepseek-reasoner` model. Since Genkit currently doesn't support it, we need to add support for it using some other mechanism. Suggest some possible approaches.
3. Does openai-go support reasoning_content already? Check the library directly. It is an indirect dependency.
4. We should rather have an openai client implemented that implements the LLMClient interface. Update the plan.
5. Currently we will only focus on DeepSeek. We don't want to migrate other OpenAI compatible providers. Update the plan.
6. Why do we need Tool-related fields in Message struct? Are they really needed? Is `ReasoningContent` not enough?
7. Ok for now, let's focus on solving the error for DeepSeek. We can revisit AppState.messages later. Update the plan.
8. Don't use "deepSeek" names in variables. In future we might move other providers.
9. Why do we need internal wire models? Can we not use already supported models from openai-go sdk? Explore it and figure out.
10. The messages for deepseek-reasoner are not streaming to the UI. Figure out why and fix it.
11. Between reasoning_content and LLM messages, there should be a new line to separate them. Implement it.
12. What is the purpose of alreadyStreamed variable? Explain it. Also add a concise comment in the code for it.
13. Currently, the `StreamChat` function in openai.go is long. There are some duplication in code too. Refactor it.
14. Now, we want to show reasoning text differently from LLM messages. How can we implement it aligning with the existing code? Suggest the most appropriate approach.