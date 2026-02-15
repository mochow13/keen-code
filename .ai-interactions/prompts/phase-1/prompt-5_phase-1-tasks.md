## Overview

This file contains the prompts used to implement the tasks in phase 1. It also has code reviews for the tasks.

## Tasks 1 & 2

### Implementation
1. Check the plan in @output-3_phase-1-plan.md and implement the very first task.
2. In logger/logger.go, you have free string checks for log levels. Let's use enum for it.
3. For task 2, we need to implement the config system. Before implementing it, explain how the configs will be setup based on user's input when user uses this CLI in their machine. For example, the user wants to add Gemini as a provider. How would they set the provider and the API key?
4. Let's simplify the config system. Instead of supporting multi-layer configs (global, project, env vars, flags), let's only support global per-provider configs. We can also avoid nested structure for llm providers. Update the @output-5_config-design.md based on this decision.
5. What if users open two sessions and want to use two different providers?
6. Let's design for two-level configs. First one is the global config which is default. Users set it using the `/provider` command in the user interface. The second one is the session-specific config which can be set up using `--provider gemini` and `--provider-api-key gemini_key`. If session-specific config is not set, then use the global config. Update the @output-5_config-design.md based on this decision.
7. Do we need any change in @internal/config/config.go?
8. We are currently supporting `temperature` as a config. But for a coding agent, we would prefer low temperature by default. So let's not support temperature as a config. Instead, we can have a default temperature for each provider. Update the @output-5_config-design.md based on this decision. Then also update the code in @internal/config/config.go.
9. Let's also remove handling of `maxTokens`. We can think about it later.
10. We don't want to have a default config at the beginning. Users have to set it explicitly using the `/provider` command in the user interface. If users don't set it, then the interface will show an error message.
11. Review the @output-3_phase-1-plan.md and @output-4_config-design.md. Based on the design, update the plan for task 2.
12. Now review both the @output-4_config-design.md and Task 2 in @output-3_phase-1-plan.md. Then figure out what to implement to support the feature.
13. No actually we just want to implement the code to get-set configs. We don't want to support it through CLI operations or REPL commands yet. Update the @output-4_config-design.md to reflect this.
14. Now review the implementation for config again. Does it support get and set correctly?
15. In @internal/config/config.go, let's use constants for provider names.
16. Write unit tests for each function for the happy paths in @internal/config/config.go. One unit test per function.
17. Let's also implement 1 unit test for success and 1 unit test for failure for each function in @internal/config/loader.go.
18. Add minimal and critical logs for the code implemented in @internal/config/config.go and @internal/config/loader.go.
19. I don't think the logger should be sent as a parameter. Let's have a local logger in each file and use it.
20. You are using `logger.Must` but that's not implemented.

### Review
1. Check the plan in @output-3_phase-1-plan.md and @output-4_config-design.md. Then also check the implementation in @internal/config/config.go and @internal/config/loader.go. Review the code and criticize it.
2. I have some code review comments for you. Let's go through them one by one.
    - First, instead of using logger in each file, let's use `slog` directly. It's a standard library and we can use it directly. We don't need to create a wrapper around it.
    - For config loader, do we really need viper? We are already using `yaml` to marshal and unmarshal the config. We can just use `yaml` to load the config as well.
    - Viper is mentioned in design docs and also in tests. Let's update those.


## Task 3 & 4
### Implementation
1. Check the plan in @output-3_phase-1-plan.md. We now want to focus on task 3. I have some specific requirements for file guard. Read the requirements in @prompt-6_file-guard-prd.md and update the plan for task 3 in @output-3_phase-1-plan.md.
2. Before implementing fileguard, let's first implement the git-awareness where we will ignore any files or directories in `.gitignore` in @internal/filesystem/gitawareness.go.
3. Ok now let's implement the fileguard in @internal/filesystem/guard.go based on the design we have.

### Review
1. Check the task 3 in @output-3_phase-1-plan.md and @prompt-6_file-guard-prd.md. Then review the code in @internal/filesystem.
2. As expected, I have some review comments for you. Let's go through them one by one.
    - We are currently blocking access to all files with `..` in the path. But that was not in the @prompt-6_file-guard-prd.md. Let's remove it since we want to allow access to files in a different project.
    - Let's block all directories that start with `~/.`.
    - Looks like we haven't really implemented recursive check for `.gitignore`. Any file that starts with `.gitignore` should be checked for gitignore rules and must be blocked.
    - The `LoadGitignore` function is taking `gitignorePath` as a parameter. This is okay. But we want to load all `.gitignore` files recursively from the root path. Let's write a helper function to find all `.gitignore` files recursively from the root path and then load them using `LoadGitignore`.

## Task 5
### Implementation
1. First, review @output_1-rfc.md. Then check the plan in @output-3_phase-1-plan.md. We now want to focus on task 5. Explain step by step what you are going to do to implement this task. Specifically focus on this task only.
2. If no global config is set, and user doesn't provide all the required session configs, then the CLI should prompt for the missing configs. For API key, it should prompt for the key but hide the input. For others, it should show the input. For example, if user provides `--provider gemini`, then it should prompt for the API key and model. If user provides `--provider gemini --model gemini-2.5-flash`, then it should prompt for the API key only.
3. In the config yaml file, let's also store multiple models per provider so that we can in future allow users to select a different model from the same provider.
4. You are currently using default provider as `anthropic`. Don't do that. On the very first run, prompt user to set up configs including provider.
5. On the very first run, the current implementation is not asking for a model to use! Let's fix it.
6. We want to change the way we are currently setting up configs using user inputs. Read @prompt_7-cli-prd.md first. Then update the design docs in @output-3_phase-1-plan.md accordingly. After that, give me a plan on how you will implement the new config setup flow from user inputs. Save the plan in @output-5_basic-cli.md.
7. Let's use a dedicated config file for provider and model mapping. And let's implement it based on the updated plan.
8. Beautify the texts shown when the keen cli runs.
9. Instead of exiting the REPL, we will continue running it for the next input. For now, if user gives some input, we will simply output it back to the user. We will implement the actual AI chat functionality later.
10. Let's beatifuy REPL input from user. It should be inside a box. The output should be inside another box with different color. Also, the prompt should be like `> `.
11. In the REPL, users should be able to use `/` to give specific commands. For now, we only want to support `/exit` and `/model` commands. `/exit` will exit the REPL. `/model` will open a prompt to select a different provider, model, and API key. If there is already an API key set, users can skip it by pressing enter. If they want to change it, they can enter a new API key.
12. The input is currently shown twice. In user input where user writes, and as output. We just want to keep the first one.
13. Let's actually remove the border from he output. Let's put the input text inside a box.
14. When users press ctrl+C or ctrl+D, we should exit the REPL. But it should be exited gracefully.


### Review
1. Kimi has implemented a basic CLI functionality for this project. The PRD is in @prompt-7-cli-prd.md and plan in @output-5_basic-cli.md. Review these documents. Ignore the actual code in @output-5_basic-cli.md. After reviewing these docs, review the current implementation that's in diff. Review the code thoroughly. Suggest best practices and idiomatic Go. Also spot any inefficiencies or overkill in the implementation.
2. Let's address some review comments. First, `GetProvider` in @configs/providers/provider.go is returning a pointer to a provider. Let's change it to return a value.
3. In @internal/config/config.go, we are right now maintaining one instance of `ProviderConfig` for each provider. We should instead use a `map[string]ProviderConfig` to store all providers. Update the code and modify the tests accordingly.
4. Let's also update the design in @.ai-interactions/outputs/phase-1/output-4_config-design.md
5. Right now, we have a `defaultModel` function that hardcodes default model for each provider. Let's not do that. Let's not have any default configs at all since we always prompt users to set up configs when running the CLI for the first time. Update the code and tests accordingly.
6. In REPL, we are currently hardcoding the directory `/Users/` in `RunREPL` function. This is not going to work in windows or linux. Let's use the home directory of the user instead.
7. Keen CLI is a coding agent. The design is in output-1_rfc.md. So far, we have implemented upto phase 1. The phase 1 plan is in output-3_phase-1-plan.md. There is also a design for config in output-4_config-design.md and a design for basic CLI implementation in output-5_basic-cli.md. Go through the code so far and evaluate how far we have come. Don't write any code.
8. Looks like we have some missing pieces. Let's fix them. First, we are checking for `.gitignore` in @internal/filesystem/gitawareness.go. But we should actually check if the name starts with `.gitignore`.
9. Write unit tests for the code in @configs/providers/loader.go. Make sure the tests cover the critical paths. Don't overdo it.
10. Let's write unit test for @internal/cli/repl.go, @internal/cli/root.go and @internal/cli/setup.go.