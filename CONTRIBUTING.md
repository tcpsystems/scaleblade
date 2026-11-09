# Contributing to scaleblade

Thank you for your interest in contributing. This document explains how to propose changes, what the project expects, and how to run the quality gate locally.

## Ways to contribute

- **Bug reports** — Open an issue with a clear title, steps to reproduce, expected vs actual behaviour, and the command output (with cookies and account data redacted).
- **Feature requests** — Open an issue describing the use case and the portal workflow it addresses.
- **Documentation** — Fixes, clarifications, or new guides are welcome.
- **Code changes** — Fixes, refactors, or new commands/tools. Keep each pull request focused on one logical change.

## Before you start

1. Check existing issues and pull requests to avoid duplication.
2. For non-trivial changes, open an issue first to discuss the approach.
3. This project targets Go 1.22+. Ensure your toolchain matches.

## Development workflow

1. Fork the repository and create a branch from `main`.
2. Make your change.
3. Run the quality gate locally (see below).
4. Open a pull request with a clear description of the change and why it is needed.
5. Address review feedback. The maintainer will merge when the gate passes and the change is acceptable.

## Quality gate

Every pull request must pass the same checks that run in CI:

```bash
make check
```

This runs, in order:

- `gofmt -l .` — source formatting (must produce no output)
- `go vet ./...` — static analysis
- `go test -race -shuffle=on ./...` — tests with race detector and shuffle
- `go build ./cmd/scaleblade ./cmd/scaleblade-mcp` — both binaries build

If `go test -race` fails because `gcc` is not available, install a C compiler or run `CGO_ENABLED=0 go test ./...` for a non-race run locally; CI always runs with the race detector enabled.

## Code style

- Follow standard Go conventions (`gofmt`, `go vet`).
- Keep functions small and focused.
- Prefer explicit error handling over panic.
- Do not add dependencies without discussion; the CLI aims to stay dependency-free outside the standard library.
- Tests use `httptest` and synthetic fixtures only. No real credentials, account data, or network calls in tests.

## Commit messages

Use conventional commits:

```
<type>(<scope>): <short summary>

<body if needed>
```

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `build`, `ci`.

Scope is the package or area affected (e.g., `cli`, `mcp`, `api`, `docs`).

Example:
```
feat(cli): add --output flag for JSON formatting

Allows consumers to request JSON output without the --json flag alias.
```

## Pull request guidelines

- Keep the diff focused. One logical change per PR.
- Update documentation when behaviour changes.
- Add tests for new functionality.
- Do not commit generated files or binaries.
- The PR title should match the commit message convention.

## Security

If you discover a security issue, follow the process in [SECURITY.md](SECURITY.md). Do not open a public issue with exploit details.

## License

By contributing, you agree that your contributions will be licensed under the MIT License (see [LICENSE](LICENSE)).