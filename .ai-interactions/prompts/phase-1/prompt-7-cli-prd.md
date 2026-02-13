We want to simplify the CLI experience. It will work this way:

- For now, users can open the CLI with `keen` command only.
- We will remove all flags from the `keen` command.
- When `keen` is opened for the first time, user will be prompted:
    - Select a provider from a predefined list using arrow keys
    - Enter API key for the selected provider
    - Select a model for that specific provider from a predefined list using arrow keys
- Internally, Keen's config management will update the API keys for the respective providers. And it will update the active provider and model.
- We will also implement a `/model` command in the REPL to select specific provider, model, and API key. If an API key already exists for the selected provider, users can just press enter to use the existing API key. If an existing API key is not set, then users will be prompted to enter the API key.