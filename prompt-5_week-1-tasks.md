Prompts used to implement the tasks in week 1. The prompts here updated the design doc for config system (@output-5_config-design.md) and the implementation (@internal/config/config.go).

1. Check the plan in @output-3_week-1-plan.md and implement the very first task.
2. In logger/logger.go, you have free string checks for log levels. Let's use enum for it.
3. For task 2, we need to implement the config system. Before implementing it, explain how the configs will be setup based on user's input when user uses this CLI in their machine. For example, the user wants to add Gemini as a provider. How would they set the provider and the API key?
4. Let's simplify the config system. Instead of supporting multi-layer configs (global, project, env vars, flags), let's only support global per-provider configs. We can also avoid nested structure for llm providers. Update the @output-5_config-design.md based on this decision.
5. What if users open two sessions and want to use two different providers?
6. Let's design for two-level configs. First one is the global config which is default. Users set it using the `/provider` command in the user interface. The second one is the session-specific config which can be set up using `--provider gemini` and `--provider-api-key gemini_key`. If session-specific config is not set, then use the global config. Update the @output-5_config-design.md based on this decision.
7. Do we need any change in @internal/config/config.go?
8. We are currently supporting `temperature` as a config. But for a coding agent, we would prefer low temperature by default. So let's not support temperature as a config. Instead, we can have a default temperature for each provider. Update the @output-5_config-design.md based on this decision. Then also update the code in @internal/config/config.go.
9. Let's also remove handling of `maxTokens`. We can think about it later.
10. We don't want to have a default config at the beginning. Users have to set it explicitly using the `/provider` command in the user interface. If users don't set it, then the interface will show an error message.
11. Review the @output-3_week-1-plan.md and @output-4_config-design.md. Based on the design, update the plan for task 2.
12. Now review both the @output-4_config-design.md and Task 2 in @output-3_week-1-plan.md. Then figure out what to implement to support the feature.
13. No actually we just want to implement the code to get-set configs. We don't want to support it through CLI operations or REPL commands yet. Update the @output-4_config-design.md to reflect this.
14. Now review the implementation for config again. Does it support get and set correctly?
15. In @internal/config/config.go, let's use constants for provider names.
16. Write unit tests for each function for the happy paths in @internal/config/config.go. One unit test per function.
