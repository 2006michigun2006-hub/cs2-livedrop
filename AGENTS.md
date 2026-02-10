# Repository Guidelines

## Project Structure & Module Organization
This repository is a small Go service for CS2 LiveDrop events.

- `cmd/server/main.go`: application entrypoint and HTTP route wiring.
- `internal/auth/`: authentication domain logic.
- `internal/gsi/`: Game State Integration request models and handlers.
- `go.mod`: module definition (`go 1.21`).
- `Assignment3_AP1.pdf`: project assignment reference.

Keep new runtime features under `internal/<domain>` and keep `cmd/` limited to composition/bootstrapping.

## Build, Test, and Development Commands
Use standard Go tooling from the repository root:

- `go run ./cmd/server`: run the API server locally on `:8080`.
- `go build ./cmd/server`: verify the server builds.
- `go test ./...`: run all tests across packages.
- `go fmt ./...`: apply canonical formatting.
- `go vet ./...`: catch common correctness issues.

Run `go fmt` and `go test` before opening a PR.

## Coding Style & Naming Conventions
Follow idiomatic Go style and let `gofmt` decide layout (tabs for indentation, standard spacing).

- Package names: short, lowercase (`auth`, `gsi`).
- Exported identifiers: `PascalCase` (`NewService`, `GameStatePayload`).
- Unexported identifiers: `camelCase`.
- File names: lowercase; use descriptive domain names (for example, `handler.go`, `service.go`).

Prefer small handlers and move reusable logic into package-level services.

## Testing Guidelines
There are currently no `_test.go` files. Add tests with each feature/fix:

- Place tests next to implementation (`internal/auth/auth_test.go`).
- Name tests as `Test<FunctionOrBehavior>`.
- Use table-driven tests for input validation and branch coverage.

Minimum expectation: cover success path and key error paths for new logic.

## Commit & Pull Request Guidelines
Git history is not available in this workspace, so no existing commit convention could be verified. Use this standard going forward:

- Commit format: `type(scope): short summary` (example: `feat(gsi): validate POST payload`).
- Keep commits focused and atomic.
- PRs should include: purpose, behavior changes, test evidence (`go test ./...` output), and sample request/response for API changes.
