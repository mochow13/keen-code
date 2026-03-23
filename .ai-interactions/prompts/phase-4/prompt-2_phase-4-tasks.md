## Phase 4 Tasks

### UI Improvements

1. Currently, we use quite bright colors for keen code UI. Check the styles in @internal/cli/repl/styles.go. Based on that, update the syles with proper material UI colors.
2. Fix thinking for genkit clients so that thinking tokens are visually handle they are done for openai compatible models.
3. Currently, model selection using up-down arrows don't wrap around. Let's fix that.
4. Review the styles in @internal/cli/repl/styles.go. Currently, the colors are all over the place. Without changing anything in the UI, organise the colors.

### Interruption

1. Let's say we interrupt a message while LLM is working on it. Then we send another message after interrupting. Right now, LLM still tries to complete the last message before interruption. Ideal behaviour will be ignore the interrupted message/task and focus on the new one. Why does it happen?
2. What if we discard the message that was interrupted from the message history?
3. Between "interruption marker" and "discarding message", which approach is followed by ../opencode and ../kimi-cli? Make a recommendation based on your research.
4. Show me the code changes required to implement the "interruption marker" approach.
5. But if we check for length in approach 1, LLM won't have the interrupted signal either.


### Miscellaneous

1. From the root of the codebase, we have `cmd/agent/main.go` file. We can only have `cmd/main.go`. Let's move the main file and remove `agent` directory. But first, explain if there are any side effects of this change.
2. Ok let's update it.
3. We also have `configs/providers/` but we want it to be `providers/` directory. First outline the side effects of this change. Then update accordingly.
4. Based on the current status of the project, what are the next most important features we should think of?


### Autosuggestion
1. Right now, when users type a `/` command, we don't autosuggest the available commands. We should have a prefix-based autosuggestion for the available commands. Some important requirements are:
  - The autosuggestion should be shown in a dropdown menu
  - When the dropdown menu renders, the input text area can move up to show the dropdown menu
  - When the dropdown menu disappears, the input text area should move down to the original position but we are flexible on this
  - The dropdown menu will be rendered based on the prefix of the input text, starting from the first character which is a `/`
  - In the dropdown menu, there will be two parts:
    - On the left side, there will be the command itself
    - On the right side, there will be the description of the command
  - Users can user up-down arrows to navigate through the dropdown menu
  - Users can press enter to select a command
  - Users can press escape to close the dropdown menu
  - Users can press tab to autocomplete the command
    - If there are multiple commands, tab will autocomplete the first one
    - Matching columns will be sorted alphabetically
    - If there are no matching commands, the dropdown menu will be closed
    - If users press tab for an unmatched command, nothing will happen—it will be ignored
Based on the requirements, create a plan for the implementation with granular todo items. Save the plan in @.ai-interactions/outputs/phase-4/output-6_autosuggestion.md.
2. Actually, users can also press enter to select a command with arrow keys. So when dropdown is shown, users cannot press enter to send a message. Rather, they can only select a command. If the dropdown disappears, then pressing enter sends a message.
3. When we press enter, the dropdown should also disappear.
4. The arrow keys should wrap around the dropdown menu. Meaning, if users press up arrow when the first command is selected, the last command should be selected. Similarly, if users press down arrow when the last command is selected, the first command should be selected.
5. Let's change the styling for the suggestions. We don't need borders. And we don't need to fill the selection. Rather, we can change the color of the currently selected command and its description text.
6. 