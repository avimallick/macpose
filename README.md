# Macpose

Macpose is a Compose-style local development runner for Apple Containers.
It lets Mac developers run simple multi-container backend and AI stacks without Docker Desktop.

Macpose is **not** full Docker Compose compatibility. It is a transparent, Compose-lite wrapper over Apple's [`container`](https://github.com/apple/container) CLI on macOS.

## What it is

- A CLI that reads `compose.yaml` / `docker-compose.yml`
- Parses a useful subset of the Compose spec
- Generates an execution plan
- Shells out to Apple `container` to create networks, volumes, build images, and run services

## What it is not

- Docker Desktop or `docker` CLI
- A complete Compose implementation (no Swarm, profiles, secrets, deploy, etc.)
- A replacement for production orchestration

## Requirements

- macOS (Apple Container targets **macOS 26+**)
- Apple silicon Mac (recommended; Apple Container is optimized for arm64)
- Apple [`container`](https://github.com/apple/container) CLI installed and running (`container system start`)
- Go 1.22+ (for building from source)

## Install from source

```bash
git clone https://github.com/avimallick/macpose.git
cd macpose
make install   # installs to $(go env GOPATH)/bin
macpose --version
```

Or build locally:

```bash
make build
./bin/macpose --version
```

## Install via Homebrew tap

```bash
brew tap avinashmallickuk/tap
brew install macpose
```

Or install directly:

```bash
brew install avinashmallickuk/tap/macpose
```

See [`Formula/macpose.rb`](Formula/macpose.rb) for the formula draft (also suitable for copying into [homebrew-tap](https://github.com/avinashmallickuk/homebrew-tap)).

## Quickstart

```bash
cd examples/fastapi-postgres-redis

macpose doctor
macpose check
macpose plan
macpose convert
macpose up -d
macpose ps
macpose logs api
macpose exec api sh
macpose down -v
```

## Commands

| Command | Description |
|---------|-------------|
| `macpose up [-d] [service...]` | Create resources and start services |
| `macpose down [-v]` | Stop containers, remove network; `-v` removes named volumes |
| `macpose ps` | List project services |
| `macpose logs [-f] [service...]` | Show logs (prefixed for multiple services) |
| `macpose exec <service> <cmd...>` | Run a command in a service container |
| `macpose build [service...]` | Build services with `build` specs |
| `macpose plan [--json]` | Show execution plan |
| `macpose check` | Compatibility report |
| `macpose doctor` | System and project readiness |
| `macpose convert` | Print equivalent `container` commands |
| `macpose version` | Print version |

### Global flags

```bash
--file, -f          Compose file
--project-name, -p  Project name (default: sanitized directory name)
--verbose           Verbose output
--dry-run           Print actions without executing
--json              JSON output (where supported)
```

## Supported Compose fields (MVP)

**Services:** `image`, `build`, `ports`, `environment`, `env_file`, `volumes`, `command`, `entrypoint`, `depends_on`, `working_dir`, `labels`, `restart` (warn), `healthcheck` (best-effort polling)

**Top-level:** `volumes`, `networks`

## Unsupported / warned fields

Reported by `macpose check`:

- `deploy`, `secrets`, `configs`, `profiles`, `scale`, `extends`
- `network_mode`, `privileged`, Docker socket mounts
- Swarm-only features

## Service discovery limitations

Apple `container` DNS resolves **container names**, not bare Compose service names.

Macpose names containers `{project}_{service}` (e.g. `vfree_db`) and injects environment variables:

```text
MACPOSE_SERVICE_DB_HOST=vfree_db
MACPOSE_SERVICE_REDIS_HOST=vfree_redis
```

Use these in your application code or connect using the generated container hostname. Bare names like `db` may not resolve — `macpose check` warns about this.

## Project naming and resources

For project `vfree`:

| Resource | Name |
|----------|------|
| API container | `vfree_api` |
| DB container | `vfree_db` |
| Network | `vfree_default` |
| Volume | `vfree_pgdata` |
| Built image | `vfree_api:latest` |

Local state is stored under `~/.macpose/projects/<project>.json`.

## Shell completions

```bash
macpose completion bash > $(brew --prefix)/etc/bash_completion.d/macpose
macpose completion zsh > ${fpath[1]}/_macpose
macpose completion fish > ~/.config/fish/completions/macpose.fish
```

Or run `make completions` to generate files under `completions/`.

## Development

```bash
make test
make lint
make fmt
make snapshot   # darwin/arm64 binary in dist/
```

## Troubleshooting

**`container` not found**

Install Apple Container and ensure `container` is in your `PATH`.

**Port already in use**

```bash
lsof -i :8000
macpose down
macpose up -d
```

**Service cannot connect to `db`**

Use `MACPOSE_SERVICE_DB_HOST` or the full container name (`{project}_db`).

**`container system` not running**

```bash
container system start
macpose doctor
```

## Roadmap

- Richer Compose coverage (profiles, health readiness, resource limits)
- Improved interactive `exec` TTY handling
- Optional DNS/helpers for bare service-name discovery
- Intel Mac guidance if Apple Container support expands

## Contributing

Contributions welcome on `main`. Please run `make lint` and `make test` before opening a PR.

## License

Apache 2.0 — see [LICENSE](LICENSE).
