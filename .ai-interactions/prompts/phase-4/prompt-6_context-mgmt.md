## Context Management

In Keen Code, we don't have any context management in place. We need to implement a fundamental context management system.

### Context Status UI

1. As the very first step, we will implement a simple context status UI for Keen Code. Below are the requirements:
  - The status  will be shown at the bottom of the screen under the input text area on the right side
  - It will have two parts: one progress bar and one percentage indicator
  - The status of the context will be determined based on the number of tokens in the current context size vs the context window of the current model. For example, if the current context size is 1000 tokens and the context window is 2000 tokens, the status will be 50%
  - The progress bar will be a horizontal bar with the current percentage filled
  - The percentage indicator will be a text showing the current percentage
  - The progress bar and the percentage indicator will be styled to match the theme of the UI
  - The progress bar and the percentage indicator will be updated in real-time as the conversation progresses based on the model's context window and the current context size
  - The progress bar and the percentage indicator will be updated in real-time as the model changes
  - We need to maintain a mapping between the model and its context size. This info can be maintained in @providers/registry.yaml for each model as a new field called `context_window`
  - To figure out the context size of a model, use web search
  - To determine the current context size, we need can use a simple assumption: 1 token is approximately 4 characters. So if there are 1000 words in the current conversation, then the current context size is 1000/0.75 = 1333 tokens.
  - Based on the requirements, create a plan for the implementation with granular todo items. Save the plan in @.ai-interactions/outputs/phase-4/output-6_context-status-ui.md.
2. Why are we rebuilding the conversation in @context_status.go using the currentContextStatus function? Why don't we just use the conversation history?
3. Actually, recalculating the context on every token is too much. Let's only update the status after each LLM message is done. So when LLM response finishes, we can update the status. It will reduce the number of times we rebuild the current context size.
4. Instead of coloring the bar for various percentages, we can color the percentage. So the bar will have one color.
5. Let's support context percentage up to 2 decimal points.
6. 