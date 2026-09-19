# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Envoy as a front door for perf and chaos testing. `gatewayctl` (a Go CLI)
adds/removes backends and toggles fault injection at runtime, without a
full xDS control plane. Three independent Go modules live in this repo:

- `gatewayctl/` - the CLI, run on the host. Renders `config/values.yaml`
  into Envoy CDS/LDS YAML and pushes it to a distribution endpoint.
- `config_server/` - a small standalone HTTP service that stores
  uploaded CDS/LDS config (the `http` distribution mode).
- `fetcher/` - a Go binary that runs *inside* the Envoy container
  (via `supervisor.ini`), polling the config server or S3 and writing
  `cds.yaml`/`lds.yaml` into `/etc/envoy/dynamic`.
- `app/` - `default_app`, the bundled echo server Envoy falls back to
  for unmatched requests.

## Commands

**Build:**
```bash
make all                          # builds the envoy-perf-gateway (Envoy) image
make -C config_server image       # builds the config_server image
make -C gatewayctl install        # go install gatewayctl onto $PATH
make build-gatewayctl-all         # cross-compile gatewayctl for darwin/linux, arm64/amd64 -> gatewayctl/dist/
```

**Test** (gatewayctl only - no tests exist elsewhere in the repo):
```bash
make -C gatewayctl test           # go test ./...
go test ./... -run TestName       # single test, run from gatewayctl/
```

**Run from source, HTTP distribution mode:**
```bash
make -C config_server up          # builds + runs config_server on :8090
make up                           # builds + runs the gateway on :8080/:9901, polling config_server
```

**Run from source, S3 distribution mode:**
```bash
make -C gatewayctl install
make all
export CONFIG_S3_BUCKET=my-bucket CONFIG_S3_PREFIX=envoy-perf-gateway/
make run-s3                       # needs AWS credentials in the shell env
```

Both HTTP and S3 modes default `API_TOKEN`/`CONFIG_API_TOKEN` to `default`
if not exported - fine for localhost only.

**gatewayctl dev loop:** `gatewayctl` is a self-contained Go module with its
own `go.mod`/`Makefile` under `gatewayctl/`; build/test/run it from there,
not from the repo root.

## Architecture

Clusters and routes are **not** static in `config/envoy.yaml`. Envoy's
bootstrap only points `dynamic_resources.cds_config`/`lds_config` at
`/etc/envoy/dynamic/{cds,lds}.yaml` with `watched_directory` set, so Envoy
hot-reloads on inotify - no xDS server, no gRPC streams.

```
gatewayctl (host)                                  Envoy container
  add-backend/remove-backend                     +-------------------------------+
        |                                         | config-fetcher (supervisor)  |
        v                                         |   polls HTTP or S3           |
  config/values.yaml -> render -> rendered/{cds,lds}.yaml |   atomic-writes into -v |
        |                                         v                          |   |
        |                              /etc/envoy/dynamic  <------------------+
        |  HTTP: gatewayctl push-http uploads to config_server's storage/    |
        |  S3:   gatewayctl push-s3 uploads rendered/ to a bucket            v
        +------------------------------------------------------> Envoy inotify watch -> hot-reload
```

Key design point: `config/dynamic` is **not** bind-mounted from the host.
On Docker Desktop for Mac, host writes into a bind mount sync file content
but don't reliably propagate the inotify event, so Envoy never notices.
Instead `fetcher/` runs *inside* the container and writes to
`/etc/envoy/dynamic` itself - a write native to the container's own
filesystem, which does trigger inotify. `gatewayctl` on the host only ever
writes to `config/values.yaml` and the local `rendered/` directory; it
never touches the container's filesystem directly. Both `gatewayctl` and
the fetcher write atomically (temp file + rename) so a reader never
observes a partial file mid-swap - see `gatewayctl/internal/atomicwrite`.

**gatewayctl internals** (`gatewayctl/internal/`):
- `config` - loads/saves `values.yaml` (the backend list: name, host,
  port, TLS, route prefix, domain).
- `render` - builds Envoy CDS/LDS structs from `config.Values` (adds the
  static `envoy_admin` and `default_app` clusters, then one cluster/route
  per backend) and marshals them to YAML.
- `envoyconfig` - the Go structs mirroring Envoy's CDS/LDS resource shape.
- `settings` - gatewayctl's own settings file (`~/.config/gatewayctl/config.yaml`
  by default, overridable via `--config`/`GATEWAYCTL_CONFIG`): distribution
  mode (`http`/`s3`) plus the config server URL/token or S3 bucket/prefix.
  Set once via `gatewayctl config set ...` so `add-backend`/`remove-backend`
  push automatically afterward.

**Distribution modes**, selected by `CONFIG_SOURCE_KIND` on the Envoy
container - both cloud-agnostic, both require an explicit push after every
change:
- `http` - `fetcher` polls `config_server` (its own Go module, own
  `storage/` dir, deployable standalone); `gatewayctl push-http` POSTs
  rendered YAML to it. Gated by `API_TOKEN`.
- `s3` - `fetcher` polls an S3 bucket; `gatewayctl push-s3` uploads
  `rendered/` there. Use this once config needs to be shared across
  multiple gateway instances.

**Routing model** (see `render.BuildCDS`/`BuildLDS`):
- `--route-prefix` on `add-backend` routes that path prefix to the
  backend, rewritten to `/` upstream. Omit it to register a cluster with
  no route.
- `--domain` gives a backend its own Envoy virtual host matched on the
  `Host` header, instead of sharing the catch-all one. Combine with
  `--route-prefix` to scope it to a path under that domain.
- Backends with neither flag still get a cluster, just no route.
- Unmatched requests fall through to `default_app` (the bundled echo
  server on `:5050`).

**Fault injection**: every route (each backend plus `default_app`) gets an
independently toggleable fault config via a unique Envoy runtime key per
`--target` (`gatewayctl fault abort|delay|reset --target <name>`), so
faulting one backend never affects another's traffic. Toggled live through
Envoy's admin API - no config reload, takes effect on the next request.
Requires the `layered_runtime.admin` layer in `config/envoy.yaml`; without
it `/runtime_modify` returns `503 No admin layer specified`.

See `docs/architecture.md` for the full picture with diagram.
