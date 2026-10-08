# Contribution Guide

Thank you for considering contributing to Go Myanmar Payments! Please review the following guidelines before submitting a pull request.

For significant changes, please open an issue first so we can discuss the approach.

## Process

1. Fork the project
2. Create a new branch
3. Code, test, commit, and push
4. Open a pull request detailing your changes

## Guidelines

- Ensure the code is formatted with `gofmt` and passes `go vet ./...`.
- Keep the module dependency-free: it uses the standard library only.
- Validation rules, amount limits and currencies must match each gateway's official documentation; link the document in your pull request.
- Send a coherent commit history, making sure each commit in your pull request is meaningful.
- You may need to [rebase](https://git-scm.com/book/en/v2/Git-Branching-Rebasing) to avoid merge conflicts.
- Please remember that we follow [SemVer](http://semver.org/); the import path carries the major version (`/v4`).

## Setup

Clone your fork; Go 1.22 or higher is the only requirement:

```bash
go version
```

## Lint

Format and vet your code:

```bash
gofmt -l .
go vet ./...
```

## Tests

Run all tests:

```bash
go test -race ./...
```

## Releasing

Pushing a version tag releases the module: `.github/workflows/release.yml` runs the test suite, creates the GitHub release, and asks the Go module proxy for the new version so pkg.go.dev lists it.

1. Add the release's section to `CHANGELOG.md` (`## v4.1.0 - 2026-10-08`). Its body becomes the GitHub release notes; when no section matches the tag, the notes are generated from the merged pull requests instead.
2. Merge into `dev`.
3. Tag the merged commit and push the tag:

   ```bash
   git tag v4.1.0
   git push origin v4.1.0
   ```

That's it. Tags must be semantic versions whose major version matches the module path (`v4.x.y` for `/v4`); a pre-release suffix such as `v4.1.0-rc.1` marks the GitHub release as a pre-release. `CHANGELOG.md` is written by hand before tagging; nothing commits it back after the release. The module proxy step is skipped while the repository is private.
