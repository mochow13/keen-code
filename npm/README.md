# Keen Code

Keen Code is a terminal-based coding agent for working on local codebases.

## Install with script

```bash
curl -fsSL https://raw.githubusercontent.com/mochow13/keen-code/main/scripts/install.sh | bash
```

To pin a specific version:

```bash
curl -fsSL https://raw.githubusercontent.com/mochow13/keen-code/main/scripts/install.sh | bash -s -- -v v0.1.4
```

Installs to `/usr/local/bin` if writable, otherwise `$HOME/.local/bin`.

## Install with `npm`

Install the CLI globally:

```bash
npm install -g keen-code
```

Check that the install worked:

```bash
keen --version
which keen
```

You can also run it without a global install:

```bash
npx keen-code --version
```

## Run Keen

Start Keen in your current directory:

```bash
keen
```
