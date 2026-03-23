## Phase 4 Tasks

### UI Improvements

1. Currently, we use quite bright colors for keen code UI. Check the styles in @internal/cli/repl/styles.go. Based on that, update the syles with proper material UI colors.
2. Fix thinking for genkit clients so that thinking tokens are visually handle they are done for openai compatible models.

### Interruption

1. Let's say we interrupt a message while LLM is working on it. Then we send another message after interrupting. Right now, LLM still tries to complete the last message before interruption. Ideal behaviour will be ignore the interrupted message/task and focus on the new one. Why does it happen?
2. What if we discard the message that was interrupted from the message history?
3. Between "interruption marker" and "discarding message", which approach is followed by ../opencode and ../kimi-cli? Make a recommendation based on your research.
4. Show me the code changes required to implement the "interruption marker" approach.
5. But if we check for length in approach 1, LLM won't have the interrupted signal either.
