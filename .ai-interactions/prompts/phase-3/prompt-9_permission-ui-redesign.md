## Permission UI Redesign

1. Currently, how is permission UI rendered?
2. We want to change that. Right now, the permission UI is rendered as a standalone interface. What if we want to show it right below outputs of different tools so that users can easily view what LLM wants to do. How should we approach this? Don't code.
3. Ok let's draft a plan and save it in @.ai-interactions/outputs/phase-3/ as output-11_permission-ui-redesign.md.
4. The plan mentions edit_file tool but it doesn't exist yet. We will implement it later. In the plan, we want to reflect that. The goal is to make the changes in a way so that we can accommodate edit_file seamlessly.
5. Implement the plan in @.ai-interactions/outputs/phase-3/output-11_permission-ui-redesign.md
6. We have too many tests in @internal/cli/repl/permission_card_test.go. We only should keep useful tests. Let's trim it.
7. Move the tests to @internal/cli/repl/streaming_test.go.
8. Now, we have a permission card. This is better than modal approach. We want to follow the same principle for model selection. The code is mainly in @internal/cli/modelselection/. Let's implement.