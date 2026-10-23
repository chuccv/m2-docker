# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

`md` is a single Go binary (cobra) that runs local Magento 2 sites with plain Docker Compose + Traefik, replacing DDEV while keeping DDEV's container names, hostnames and DB volumes. User docs: `README.md`. Design decisions and why: `docs/design.md`, `docs/plan.md` (Vietnamese).

## Commands

```bash
go test ./...                                     # all tests, no Docker needed
go test ./internal/ui -run TestSitesView -v       # single test
go vet ./...
go build -o /home/neil/.local/bin/md ./cmd/md     # install the binary in use on this machine
```

After rebuilding, a running `md ui` (port 9900) still serves the old binary until it is restarted.

## Architecture

- `internal/cmd` — cobra commands. Everything hangs off `env` (`cmd.go`): runner, scan roots, home dir, I/O. `up.go` is the core flow; `stop.go`, `migrate.go`, `install.go`, `ui.go`, `mcp.go` call into the same `env` methods (`upByName`, `stopByName`, `restartByName`), so CLI, web UI and MCP share one code path.
- `internal/run` — `Runner` interface wrapping every external command (`docker`, `mkcert`, ...). Tests use `run.FakeRunner` (canned output/errors keyed by the full command line, records `Calls`). All Docker interaction goes through it; there is no Docker API client.
- `internal/site` — loads a site from `<root>/.md/config.yaml`, falling back to `.ddev/config.yaml` (`.md` wins, `.ddev/` is never written). `support.go` `Supported` is the single gate for what md can run (magento2 only, nginx/apache, no rabbitmq, search version never `latest`).
- `internal/compose` — renders compose YAML: per-site (`site.go`, project `md-<name>`, file `~/.md/sites/<name>/compose.yaml`) and shared (`shared.go`, project `md-shared`: `md-router` Traefik, `md-xhgui`, one `md-opensearch-<v>` / `md-elasticsearch-<v>` per version). Golden files in `testdata/`.
- Shared containers are attached to each site network with `docker network connect --alias` (`connectShared` in `up.go`); `stack.Down` disconnects them before `compose down`.
- `internal/router`, `internal/tls` — Traefik file-provider config per site and mkcert certs.
- `internal/install` — `md install` (blank folder only) and `md migrate` (DDEV site → writes `.md/config.yaml`, pins search version, then `up`). Stack defaults per Magento version in embedded `stacks.yaml`, overridable by `~/.md/stacks.yaml`.
- `internal/website` — multi-website/store: renders nginx/apache run-code maps and locks base URLs into `env.php` via `config:set --lock-env`.
- `internal/ui` — `md ui`: stdlib HTTP server, embedded `web/` (plain JS, no build step), live stats over SSE from one `docker stats`/`docker events` stream. POSTs require header `X-MD: 1` and loopback Origin. Actions go through `uiActions` in `internal/cmd/ui.go`, serialized by an in-process mutex.
- `images/php` — embedded Dockerfile context for `md/php:<version>`; `up` rebuilds only when the context hash label changes.
- State: `~/.md/state.json` (known sites + search versions in use). `up`/`stop`/`restart`/`poweroff` hold an exclusive `flock` on `~/.md/md.lock` (`lockHome` in `internal/cmd/state.go`); never call one locked entry point from inside another.

## Contracts other tools depend on

Do not change without updating QA tooling (`qa-agent-e2e`, `agent-site.sh`, skills):

- Container names `ddev-<name>-web|db|redis`; DB user/password/database `db`/`db`/`db`; web uid 1000, Magento root `/var/www/html`.
- Hostnames `<name>.md.site` (base URL, needs `/etc/hosts` via `md hosts --all`) with `<name>.ddev.site` kept as an alias; DB volume `<name>-mariadb`, Redis volume `ddev-<name>_redis` reused as-is for sites with `.ddev/` (`<name>-redis` for sites made by `md install` / `md init`).
- Never delete DB volumes automatically; `md clean` deletes only after the user types `yes`.

## Conventions

- No comments in code.
- User-facing messages are in English.
- Any action against a real site under `/var/www/html/*` requires a claim: `AGENT_NAME=<name> /home/neil/brain/tools/agent-site.sh claim <site> "<task>"`, then `release`.

## Release

- A bug fix that passes `go test ./...` is committed and pushed to `main` without asking Neil (Neil 2026-10-23).
- At most one release per day: batch the day's fixes into a single release (Neil 2026-10-23). Until then, rebuild `/home/neil/.local/bin/md` locally to verify.
- Release: `GOOS=linux GOARCH=amd64|arm64 CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=vX.Y.Z" -o md-linux-<arch> ./cmd/md`, `sha256sum md-linux-* > SHA256SUMS`, `gh release create vX.Y.Z --target main` with the three files; then reinstall `/home/neil/.local/bin/md` and restart `md ui`.
