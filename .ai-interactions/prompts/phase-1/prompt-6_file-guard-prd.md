For file guard, we want to implement the following features:
- Working directory should be accessible to the CLI for reading by default. But for writing, CLI must ask for permission from the user.
- If users wants it to access a directory or file not in the working directory, CLI must ask for permission from the user.
- CLI cannot access any directory or file in `.gitignore`.
- CLI should not be able to access any directory or file in `~/.ssh`, `/etc`, `~/.aws`, `/usr`, and other sensitive directories.