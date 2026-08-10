---
name: release
description: Release a new version of the project.
---

# Release Skill

Target version (optional): $ARGUMENTS

If no target version is provided, inspect the latest release tag and commits since that tag before making any changes. Suggest a semantic-version bump to the user and wait for confirmation:

- **major** for intentional breaking changes.
- **minor** for backward-compatible features.
- **patch** for fixes, documentation, dependencies, and internal changes.

Use `git tag --sort=-version:refname` to identify the latest version tag and `git log <latest-tag>..HEAD --oneline` to review subsequent commits. Also consider unreleased changelog entries. State the latest tag, the commits considered, the recommended version, and a concise rationale. If there are no commits since the latest tag, say that no release is recommended.

Once a version is supplied or confirmed, use it as the target version for the remaining steps.

## Steps

1. **Bump versions.** Update the version string in both files to the target version:
   - `cmd/main.go`
   - `npm/package.json`

2. **Update `CHANGELOG.md`.**
   - Move all entries under `[Unreleased]` into a new `[X.Y.Z] - YYYY-MM-DD` section below it.
   - Add a new empty `[Unreleased]` section at the top.
   - Check the commit history for any changes that should be included in the release and were missing in the [Unreleased] section.
   - Update the comparison links at the bottom of the file.

3. **Run the tests.**
   ```bash
   go test -race ./...
   ```

4. **Verify the npm wrapper.**
   ```bash
   cd npm && npm pack --dry-run
   ```

5. **Commit the version bump.** Stage only the four changed files and commit:
   - `cmd/main.go`
   - `npm/package.json`
   - `CHANGELOG.md`

6. **Create and push the tag.**
   ```bash
   git tag vX.Y.Z && git push origin main && git push origin vX.Y.Z
   ```
   The tag must match `npm/package.json` version exactly (e.g. `v1.2.3`).

7. **Watch the pipelines.** Pushing the tag triggers the build and release workflows. Watch them to completion:
   ```bash
   gh run watch $(gh run list --limit 1 --json databaseId --jq '.[0].databaseId')
   ```
   If a run fails, inspect logs with `gh run view <run-id> --log-failed`.

8. **Confirm.** Once both pipelines succeed, report the tag, commit SHA, and release URL (`gh release view vX.Y.Z --web` or `--json url`) to the user.
