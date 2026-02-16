1. We need to refactor the code in @internal/cli/repl.go. It's too big right now. Give me some ideas on how to refactor these. Save the plan in @.ai-interactions/outputs/phase-2/output-6_repl-refactor.md.
2. Ok so we will now address the refactoring ideas in @.ai-interactions/outputs/phase-2/output-6_repl-refactor.md. This is critical change so we will do it one by one. Do the first one and ask for my approval before moving to the next one. I will test the changes in between.
3. We should write unit tests for functions in @internal/cli/streaming.go. Which functions are worthy enough? And how would you write the tests for them given that there is bubbletea involved?
4. Add unit tests for @internal/cli/streaming.go.
5. Proceed to refactoring with the next step. Make sure unit tests work accordingly after refactor.
6. Let's implement the next step: `Move Model Selection to internal/cli/modelselection/`.
7. Based on the refactoring we did so far, let's do some code review:
    - Refactor the `View()` function in @internal/cli/modelselection/model.go. It's big right now.