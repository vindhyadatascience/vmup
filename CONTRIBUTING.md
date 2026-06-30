# Contributing to vmup

Thanks for your interest in contributing! This document covers how to build,
test, and submit changes.

## Prerequisites

- [Go](https://go.dev/dl/) 1.26 or newer
- [Google Cloud SDK](https://cloud.google.com/sdk/docs/install) (`gcloud`) — required to run the app
- A POSIX shell and `make` (optional, for the `Makefile` targets)

## Building and running

```bash
make build      # build ./vmup for the current platform
make run        # build and run
go vet ./...    # static checks
go test ./...   # tests (when present)
```

You can also build directly with `go build -o vmup .`.

## Project layout

- `main.go`, `embed.go` — entry point and embedded Terraform assets
- `assets/` — embedded Terraform (`*.tf`) provisioned at runtime
- `internal/tui/` — Bubble Tea TUI screens and components
- `internal/gcloud/` — gcloud CLI / Compute API integration
- `internal/config/` — settings and `terraform.tfvars` read/write
- `internal/platform/` — environment detection helpers

## Making changes

1. Fork the repository and create a topic branch.
2. Keep changes focused; match the surrounding code style (`gofmt`/tabs for Go).
3. Run `go vet ./...` and `go build ./...` before opening a PR.
4. Write a clear PR description explaining the motivation and the change.

## Commit messages

Use a concise imperative subject line (e.g. "Add data-disk resize support"),
followed by a body explaining the why when the change is non-trivial.

## Reporting bugs and requesting features

Open an issue using the provided templates. For security issues, please follow
[SECURITY.md](SECURITY.md) instead of opening a public issue.

## License

By contributing, you agree that your contributions will be licensed under the
[Apache License 2.0](LICENSE).
