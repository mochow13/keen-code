1. Instead of writing tests at the end in @.ai-interactions/outputs/phase-2/output-3_stream-chat-todo.md, let's put todo item for writing unit tests after each section where applicable.
2. Remove documentation part from the todo list. We will add documentation later.
3. Let's implement first 4 todo items from @.ai-interactions/outputs/phase-2/output-3_stream-chat-todo.md.
4. The current implementation of streaming in @internal/llm/genkit.go is quite different from what I see in genkit example here: https://github.com/firebase/genkit/blob/main/go/samples/basic-structured/main.go. Let's use the genkit example as reference to implement streaming.
5. How is the message sent to llm and how is it sending the response? Explain it to me.
6. The current implementation has a `streamFunc` type for the `GenerateStream` function. Why is it needed? We already have `g` which is a `*genkit.Genkit` instance. Can we use it directly instead of defining a new type? Explain why it's needed.
7. 