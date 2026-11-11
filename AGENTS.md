# tcpsystems/scaleblade — Contributor & Agent Guide

`scaleblade` is the dependency-free Go CLI (`cmd/scaleblade`) and Model Context Protocol server (`cmd/scaleblade-mcp`) for the Scaleblade gaming and infrastructure portal.

## Operating Rules

1. **Zero External Dependencies**:
   - `go.mod` must remain pure Go standard library (`CGO_ENABLED=0`). Do not add third-party modules.
2. **Quality Gate**:
   - Run `go vet ./...`, `go test -race ./...`, `staticcheck ./...`, and `govulncheck ./...` before every commit.
   - Format all Go sources with `gofmt`.
3. **Security & Credentials**:
   - Never log, echo, or commit bearer tokens, session cookies, or customer identifiers.
   - All HTTP requests must carry bounded timeouts via `context.Context`.
4. **Branch & Identity Model**:
   - Default branch is `develop`; release branch is `production`.
   - Commit as `Petar Markov <petar@petarmarkov.co.uk>` using Conventional Commits.
