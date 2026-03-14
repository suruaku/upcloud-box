# Upcloud Box v1 Implementation Checklist and Minimal Config Schema

This document locks the v1 implementation plan for a Go + Cobra CLI that provisions one secure Docker host on UpCloud and deploys a single container.

## Implementation Checklist

### Day 0: Project bootstrap
- Initialize module with `go mod init`.
- Add Cobra scaffold (`main.go`, `cmd/root.go`).
- Create command stubs: `init`, `provision`, `deploy`, `status`, `destroy`.
- Add global `--config` flag (default `upcloud-box.yaml`).

### Day 1: Config and state foundation
- Define config structs and loader with strict validation.
- Define state structs and read/write helpers for `.upcloud-box.state.json`.
- Implement `init` to generate a minimal config template and a default `cloud-init.yaml`.
- Add clear validation errors (missing credentials, bad port format, missing cloud-init path).

### Day 2: UpCloud client and provision workflow
- Implement UpCloud API wrapper for: create server, get server, delete server.
- Implement polling for server readiness and public IP assignment.
- Implement cloud-init pass-through loader (file exists, non-empty, raw bytes unchanged).
- Implement `provision` and persist state (`server_uuid`, `public_ip`).

### Day 3: SSH checks and deploy
- Implement SSH runner utility with timeout and retry.
- Add post-provision checks: SSH reachable and `docker info` succeeds.
- Implement `deploy` single-container flow:
  - Pull new image.
  - Capture previous running image for rollback.
  - Replace container (same name/ports/env).
  - Run health check loop.
  - Roll back automatically on failure.
- Persist `last_successful_image` in state.

### Day 4: Status, destroy, and UX hardening
- Implement `status` (infra + container + health summary).
- Implement `destroy` (delete by UUID from state, handle already-missing resources gracefully).
- Improve error handling for auth, quota, network, SSH, and health timeouts.
- Add concise logs and optional `--verbose` mode.

### Day 5: Reliability and docs
- Add unit tests for config/state/validation/deploy decision logic.
- Add integration smoke script: `init -> provision -> deploy -> status -> destroy`.
- Write README quickstart and troubleshooting notes.
- Freeze v1 scope and cut release candidate.

## Minimal Config Schema (`upcloud-box.yaml`)

```yaml
project: "my-app"

upcloud:
  zone: "fi-hel1"
  plan: "1xCPU-2GB"
  template: "Ubuntu Server 24.04 LTS"

provision:
  cloud_init_path: "./cloud-init.yaml"
  hostname: "my-app-prod"

ssh:
  user: "ubuntu"
  private_key_path: "~/.ssh/id_ed25519"
  connect_timeout_seconds: 120

deploy:
  container_name: "my-app"
  image: "ghcr.io/acme/my-app:latest"
  port: "80:8080"
  env_file: ".env.prod"
  healthcheck_url: "http://localhost:8080/health"
  healthcheck_timeout_seconds: 60
  healthcheck_interval_seconds: 3
```

## Minimal State Schema (`.upcloud-box.state.json`)

```json
{
  "server_uuid": "",
  "public_ip": "",
  "last_successful_image": "",
  "last_deployed_at": ""
}
```

## Environment Variables

```bash
export UPCLOUD_TOKEN="ucat_..."
```

The CLI uses `UPCLOUD_TOKEN` for UpCloud API authentication.

## Init Command Defaults

- `upcloud-box init` creates three files: `upcloud-box.yaml`, `.upcloud-box.state.json`, and `cloud-init.yaml`.
- Generated cloud-init defaults disable password SSH auth, keep root disabled, create one sudo user, and install baseline packages (`ca-certificates`, `curl`, `fail2ban`, `ufw`, `docker.io`).
- Provide SSH public key files at init time with repeatable `--ssh-key` flags.
- Override the generated username with `--user` and cloud-init output path with `--cloud-init-path`.

Example:

```bash
upcloud-box init --ssh-key ~/.ssh/id_ed25519.pub --user deploy
```

## Suggested Go Package Layout

- `cmd/`: Cobra commands (`init.go`, `provision.go`, `deploy.go`, `status.go`, `destroy.go`)
- `internal/config/`: config structs, loader, validation
- `internal/state/`: state persistence
- `internal/upcloud/`: UpCloud API client wrapper
- `internal/ssh/`: SSH command executor
- `internal/deploy/`: deploy + rollback orchestration
- `internal/health/`: health-check loop
