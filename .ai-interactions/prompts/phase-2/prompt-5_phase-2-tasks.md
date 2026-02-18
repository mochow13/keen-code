## LLM Integration and `StreamChat`

1. Instead of writing tests at the end in @.ai-interactions/outputs/phase-2/output-3_stream-chat-todo.md, let's put todo item for writing unit tests after each section where applicable.
2. Remove documentation part from the todo list. We will add documentation later.
3. Let's implement first 4 todo items from @.ai-interactions/outputs/phase-2/output-3_stream-chat-todo.md.
4. The current implementation of streaming in @internal/llm/genkit.go is quite different from what I see in genkit example here: https://github.com/firebase/genkit/blob/main/go/samples/basic-structured/main.go. Let's use the genkit example as reference to implement streaming.
5. How is the message sent to llm and how is it sending the response? Explain it to me.
6. The current implementation has a `streamFunc` type for the `GenerateStream` function. Why is it needed? We already have `g` which is a `*genkit.Genkit` instance. Can we use it directly instead of defining a new type? Explain why it's needed.
7. Let's check task 5. Is it already implemented? If yes, let's write unit tests and update the todo list.
8. For task 6, we want to support this functionality:
    - When users press enter, the input will be sent and logged (just like how it works)
    - When users press `ctrl+enter`, then a new line will be appended to the input (so users will write input now on a new line)
    - Users can copy paste text in the text box including new lines
    How would you solve this? Explain the changes needed.
9. It is not working as expected.
    - Adding a new line with ctrl+enter doesn't work
    - Sending a message with pressing enter doesn't echo it back
    - it behaves buggy overall
    Let's fix it. 
10. The text is still not echoed back. Also, there are three `>` in the screen every time. Let's align the all the new lines in the input to the first line. So it should begin aligning with the first line after `>`.
11. Ok now let's support `ctrl+j` for appending new lines to the prompt.
12. We want the text area for input to have a fixed hieght of 1 initially and a width expanding the terminal. When users put new line, the area's height should increase accordingly. And it should only be clipped if the height is more than 10.
13. Let's support `/model` command in the new implementation of `bubbletea`-based TUI. It should work similar to the previous version.
14. We should also handle first-time model selection using `bubbletea`. Let's implement it and remove dead code like `handleInput`.
15. Let's put the output in a box with heigh equal to the height of the output and width equal to the window width. 
16. Read the stream chat prd in @.ai-interactions/prompts/phase-2/prompt-3_stream-chat.md and todo items in @.ai-interactions/outputs/phase-2/output-3_stream-chat-todo.md. We want to focus on the task 7. Check the code in @internal/cli/ and @internal/llm/. Finally, explain what you would do.
17. Looks good. Let's implement it.
18. It seems API key is being stored in ~/.keen/config inside `[]` as a string. Explain it to me. Then fix it.
19. Now, the output is expanding beyond the window width. How would you fix it?
20. You are now not wrapping, you are truncating the text.
21. For the first message, I am correctly getting LLM response but for the second onwards, it's not working. There is always the error: `Error: Error 400, Message: Please use a valid role: user, model., Status: INVALID_ARGUMENT, Details: []`. What's the problem? Let's fix it.
22. Until the first message comes to the repl output, we should show a loading spin. How would you do it? First explain your approach and then implement it.
23. We want to support markdown rendering in the repl output. How to solve this? Share possible approaches.
24. What are the exact changes will be if we use `glamour`?
25. Do we have to separately detect markdown formats? Why not render the whole output as markdown?
26. We are updating three places for markdown rendering. Can this be improved?
27. Let's implement the changes in `streaming.View()` and `repl.View()` methods. Also plan for a way so that we don't duplicate markdown rendering logic in these two places.
28. I cannot type on the text area for some time and then this text appear: `11;rgb:1010/1212/1616\`. And then I can type. But code doesn't render as expected in markdown. Let's fix it.
29. It seems we cannot scroll up or down in keen-code UI right now. So if a text output is long, I can no longer scroll up to see the actual input or previous history. What's the issue specifically?
30. Review the relevant code in @repl.go and @handlers.go for scrolling in the CLI.
31. When content is rendering in chunk, the input text area flickers at the bottom. What's the issue? How would you fix it?
32. After content rendering is finished, a portion of the last few lines are overlapping with the input text area and cannot be scrolled down. Explain the reason for this issue.
33. Would the content correctly render as markdown inside viewport?
34. There are some deprecated code usages. For example, `msg.Type`, `tea.MouseWheelUp/Down`. Let's fix them and use the supported APIs.
35. We cannot copy paste in the text area. How to fix it?
36. We want to reorganise the cli package. There is only `modelselection` right now. How would you approach reorganising it?