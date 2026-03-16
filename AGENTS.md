# AGENTS.md

## Purpose
Guidance for agentic coding agents working in this repository.
Follow this file unless a user request explicitly overrides it.

## Project Snapshot
- Language: Go
- Module: `github.com/suruaku/upcloud-app-platform`
- Go version: `go 1.25.7` (`1.25.x` in CI)
- App type: Cobra CLI
- Entry point: `main.go`
- Main directories: `cmd/`, `internal/`

## Rule Files (Cursor/Copilot)
Checked for repository-specific rule files:
- `.cursorrules`
- `.cursor/rules/*`
- `.github/copilot-instructions.md`

Current status: none of these files exist in this repository.
If they are added later, treat them as higher-priority local agent instructions.

## Repository Map
- `main.go`: top-level execute, stderr printing, exit code handling.
- `cmd/`: user-facing commands (`init`, `provision`, `deploy`, `up`, `status`, `destroy`).
- `internal/config`: config schema, defaults, YAML load/validate helpers.
- `internal/state`: local state JSON load/save and deploy metadata.
- `internal/infra`: provider interface and UpCloud implementation.
- `internal/ssh`: SSH runner abstraction and path resolution.
- `internal/deploy`: container deploy workflow, health checks, rollback logic.
- `internal/health`: HTTP readiness polling over SSH.
- `.github/workflows/ci.yml`: canonical CI checks.

## Build/Lint/Test Commands
Run commands from repository root.

### Dependencies
```bash
go mod download
```

### Build
```bash
go build ./...
```

Release-style local build (matches release workflow ldflags pattern):
```bash
go build -trimpath -ldflags "-s -w -X github.com/suruaku/upcloud-app-platform/cmd.appVersion=dev" -o dist/upcloud-app-platform .
```

### Lint / static checks
```bash
go vet ./...
```

### Tests (all)
```bash
go test ./...
```

### Single-package tests
```bash
go test ./cmd
go test ./internal/config
go test ./internal/deploy
```

### Single test by name
```bash
go test ./... -run '^TestConfigValidate$'
```

### Single subtest by name
```bash
go test ./... -run '^TestDeployRun$/rollback_on_health_failure$'
```

### Single test with verbose output
```bash
go test -v ./internal/deploy -run '^TestDeployerRun$'
```

### Useful optional checks
```bash
go test -race ./...
go test ./... -cover
```

Current note: this repo currently has no `*_test.go` files.
Still use the single-test patterns above when new tests are added.

## Recommended Local Workflow
Run before opening a PR:
```bash
go mod tidy
go vet ./...
go test ./...
go build ./...
```

Minimum required to mirror CI:
```bash
go vet ./...
go test ./...
```

## Code Style Guidelines

### Formatting and file hygiene
- Run `gofmt` on every changed Go file.
- Keep imports grouped by standard `gofmt` ordering.
- Keep files ASCII unless Unicode is already required.
- Do not add comments unless logic is non-obvious.
- Prefer small focused functions over broad utilities.

### Imports
- Use explicit imports; never use dot imports.
- Alias imports only for clarity/collisions (`sshrunner`, `deployrunner`, `upcloudapi`).
- Remove unused imports and dead code in the same change.

### Types and package boundaries
- Prefer concrete structs for payload/config/state types.
- Export only what must cross package boundaries.
- Keep YAML/JSON tags consistent with existing snake_case schema.
- Use interfaces at boundaries (`infra.Provider`), not everywhere.
- Keep orchestration in `cmd/` and domain logic in `internal/*`.

### Naming conventions
- Exported identifiers use `CamelCase`; internal identifiers use `camelCase`.
- Keep Cobra command vars consistent (`initCmd`, `upCmd`, `deployCmd`).
- Use concise receiver names (`r`, `d`, `p`, `s`) matching existing code.
- Keep established error-kind constants stable once introduced.

### Error handling
- Return errors; do not panic in normal control flow.
- Wrap with context using `%w`.
- Keep user-facing messages actionable and concise.
- Keep detailed chains available for verbose/debug output.
- Validate inputs early (`TrimSpace`, required checks, range checks).
- Use `errors.Is` for sentinel cases (`os.ErrNotExist`, `context.DeadlineExceeded`).

### Context, timeouts, and retries
- Pass `context.Context` through network and remote operations.
- Bound waits and health checks with explicit timeouts.
- Retry only where transient failures are likely.
- Stop timers when exiting select branches early.

### CLI UX and state safety
- Keep output deterministic and easy to scan.
- Preserve spinner behavior and `--no-spinner` / non-TTY behavior.
- Preserve `--verbose` semantics (extra detail only when requested).
- Avoid silent changes to command flags or command behavior.
- Treat `.upcloud-app-platform.state.json` as authoritative local state.
- Keep secure write modes (`0o600`) for config/state files.

### Remote command execution
- Use `internal/ssh.Runner` rather than ad-hoc SSH shelling.
- Shell-quote user-derived values before remote command construction.
- Prefer idempotent remote operations when practical.

## Agent Working Practices
- Make minimal diffs and avoid opportunistic refactors.
- Match patterns from nearby files before introducing new abstractions.
- Update `README.md` when behavior or CLI usage changes.
- Prefer table-driven tests when adding test coverage.
- Never commit real secrets, tokens, or private keys.
