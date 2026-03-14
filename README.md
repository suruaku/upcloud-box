# upcloud-box

`upcloud-box` is a Go CLI for provisioning one secure Docker host on UpCloud and deploying one container workload.

## Install

Install with Homebrew (macOS arm64):

```bash
brew tap suruaku/tap
brew install upcloud-box
```

Homebrew installation also sets up shell completions automatically.

## Use

Requirements:

- UpCloud account and API token
- Local SSH keypair (public key for cloud-init, private key for SSH access)

Quickstart:

1) Export your UpCloud token:

```bash
export UPCLOUD_TOKEN="ucat_..."
```

2) Put your stack in `docker-compose.yaml` (or `compose.yaml`) in the project root.

3) Run full runtime flow:

```bash
upcloud-box up
```

On first run, `upcloud-box up` bootstraps `upcloud-box.yaml` automatically, then continues provisioning/deploy.

SSH key behavior:

- If `ssh.private_key_path` is empty, upcloud-box auto-detects `~/.ssh/id_ed25519`, then `~/.ssh/id_ecdsa`, then `~/.ssh/id_rsa`.
- By default, cloud-init is generated internally (no `cloud-init.yaml` file needed).
- Internal cloud-init key material is auto-detected from `.pub` keys in the same order.
- If `ssh.private_key_path` is explicitly set to an invalid path, commands fail fast.
- If no public SSH key is found, provisioning fails with guidance to pass `--ssh-key` or create a default key.

First run creates:

- `upcloud-box.yaml`
- `.upcloud-box.state.json`

`upcloud-box up` provisions the server if needed, then deploys your Docker stack automatically.
If no compose file is found, it falls back to single-container settings in `upcloud-box.yaml`.

4) Inspect status:

```bash
upcloud-box status
```

5) Clean up:

```bash
upcloud-box destroy --yes
```

This removes the tracked server and clears local infra state.

Core commands:

- `upcloud-box init` - optional manual scaffold for config/state (`--write-cloud-init` for a cloud-init file)
- `upcloud-box provision` - create server and persist infra state
- `upcloud-box deploy` - deploy your Docker stack (or single-container fallback)
- `upcloud-box up` - provision if needed, then deploy your stack
- `upcloud-box status` - local state + remote infra/app summary
- `upcloud-box destroy` - remove the server and clean state

Useful flags:

- `--config <path>`: custom config path (default: `upcloud-box.yaml`)
- `--verbose`: show detailed error output and verbose logs
- `--no-spinner`: disable spinner progress output

## Troubleshooting

- `initialize provider failed (auth)`: verify `UPCLOUD_TOKEN` is set and valid.
- `... failed (quota)`: check UpCloud resource limits and selected zone capacity.
- `post-provision checks failed (ssh)`: confirm `ssh.user` and SSH key setup match. If `ssh.private_key_path` is empty, upcloud-box auto-detects `~/.ssh/id_ed25519`, then `~/.ssh/id_ecdsa`, then `~/.ssh/id_rsa`; if it is set to an invalid path, the command fails fast.
- `read cloud-init failed (validation)`: create a public key at `~/.ssh/id_ed25519.pub` (or `id_ecdsa.pub` / `id_rsa.pub`) or run `upcloud-box init --write-cloud-init --ssh-key <path>`.
- `deploy container failed (health)`: verify app startup, exposed port mapping, and `deploy.healthcheck_url`.
- `status` shows server missing: run `upcloud-box up` to reprovision or `upcloud-box destroy --yes` to clean state.

## Development

Requirements:

- Go 1.25+

Local workflow:

```bash
go test ./...
go build ./...
```

## Release

Release artifacts are published on version tags (`v*`) for:

- Linux: amd64, arm64
- macOS: arm64

Release checklist:

1) Prepare and merge changes to `master`.
2) Create and push a version tag:

```bash
git tag -a v1.0.1 -m "v1.0.1"
git push origin v1.0.1
```

3) Wait for GitHub Actions:
- `Release` workflow publishes binaries and checksums to the GitHub Release.
- `Update Homebrew Tap` workflow opens a PR in `suruaku/homebrew-tap`.

4) Merge the Homebrew tap PR.

5) Verify install/upgrade:

```bash
brew update
brew upgrade upcloud-box
upcloud-box --version
```

Notes:

- Version tags must match `v*` (for example `v1.0.1`).
- Ensure `HOMEBREW_TAP_TOKEN` is configured in this repo's Actions secrets.
