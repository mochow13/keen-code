### Model Selection

This is the phase 2 for the development of `keen-cli`—a CLI-based coding agent. We will kick off this phase with support for the `/model` command.

Right now, only `/exit` command is implemented in the REPL. We need to implement `/model` command. We have the following requirements for this command:

1. The command will prompt users to select a provider, model, and API key. The same code that is used to configure these when users open the CLI for the first time should be reused here.
2. After the user selects the provider, model, and API key, the command should save the configuration to the config file. And the selected provider and model will be put as the `active_provider` and `active_model` in the config file and loaded in-memory for the current session.
3. The command should be accessible from the REPL by typing `/model`.
4. For provider and model, users can select it with arrow keys and press enter to select. For API key, users will copy paste and press enter to save. Of course, it will be masked.
5. If selected provider already has an API key, users can just press enter without providing any API key. In that case, the existing API key will be used.
6. If no API key is provided, the prompt must show an error message and ask the user to provide an API key again, until user provides an API key.
7. The config will only be saved if all three configs are available.

### Help Command

We need to implement `/help` command. For now, it will show all the available commands in the REPL. The output should be formatted nicely using lipgloss.

