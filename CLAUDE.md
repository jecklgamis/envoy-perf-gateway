# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Envoy as a front door for perf and chaos testing. `gatewayctl` (a Go CLI)
adds/removes backends and manages fault injection, without a full xDS
control plane. Three independent Go modules live in this repo:

- `gatewayctl/` - the CLI, run on the host. Renders `config/values.yaml`
  into Envoy CDS/LDS/runtime YAML and pushes it to a distribution
  endpoint.
- `config_server/` - a small standalone HTTP service that stores
  uploaded CDS/LDS/runtime config (the `http` distribution mode).
- `fetcher/` - a Go binary that runs *inside* the Envoy container
  (via `supervisor.ini`), polling the config server or S3 and writing
  `cds.yaml`/`lds.yaml`/`runtime.yaml` into `/etc/envoy/dynamic`.
- `app/` - `default_app`, the bundled echo server Envoy falls back to
  for unmatched requests. Response shape mirrors httpbin's `/anything`
  (`args`/`data`/`headers`/`method`/`origin`/`url`).

`charts/` holds two independent Helm charts: `envoy-perf-gateway` (the
gateway only) and `envoy-perf-gateway-config-server` (standalone,
mirroring the fact `config_server` can run on its own). `docs/index.html`
is the published user guide (GitHub Pages, `main`/`docs`); most
user-facing detail (routing, fault injection, Helm deployment,
troubleshooting) lives there rather than in this repo's own docs, so
check it before re-deriving CLI behavior from source.

## Commands

**Build:**
```bash
make all                          # builds the envoy-perf-gateway (Envoy) image
make -C config_server image       # builds the config_server image
make -C gatewayctl install        # go install gatewayctl onto $PATH
make build-gatewayctl-all         # cross-compile gatewayctl for darwin/linux, arm64/amd64 -> gatewayctl/dist/
```
Bare `make` in the repo root or `config_server/` builds that component's
image by default (not a help/usage printout). Bare `make` in `gatewayctl/`
builds the binary.

**Test** (gatewayctl only - the other three modules have no test files):
```bash
cd gatewayctl && go test ./...              # all packages
go test ./... -run TestName                 # single test, from gatewayctl/
go test ./... -cover                        # with coverage
```
Coverage is high on the pure logic (`internal/render` ~92%, `internal/envoyconfig`
100%) and lower on `cmd` (~35%, since most of it is Cobra `RunE` closures
doing file/network I/O - those are covered by manual integration testing
against a real config_server instead of mocks).

**Integration test** (`make integration-test`, or `./scripts/integration-test.sh`
directly, also runs in CI on every PR/push touching the gateway; needs
Docker, no AWS credentials or cloud access required):

- **http-mode phase**: renders a `values.yaml` covering every `add-backend`
  feature (TLS, HTTP/2, host-rewrite, domain/path-prefix routing, gzip
  compression), bakes it into a real image, boots it, and checks `cds`/`lds`
  `update_rejected`/`update_failure` are `0` via the admin API - unit tests
  can't catch a config that compiles and marshals fine but Envoy still
  rejects at listener-load time (see the compressor gotcha above; this
  script exists specifically because that bug shipped without one). Also
  does functional checks (gzip applies/doesn't leak, routes isolated).
- **S3-mode phase**: proves `gatewayctl push-s3` and the in-container
  fetcher's S3 poller actually work end to end - push → fetcher poll →
  Envoy hot-reload → real traffic - against a local `quay.io/minio/minio`
  standing in for AWS S3 (no cloud dependency, no CI secrets). Requires
  `S3_FORCE_PATH_STYLE=true` and `AWS_ENDPOINT_URL_S3` (the fetcher's/
  `gatewayctl`'s S3 client honors both - see `newS3Client` in
  `gatewayctl/cmd/root.go` and `fetcher/main.go`'s `buildSource`); real AWS
  S3 needs neither. This phase is **deliberately tolerant** of a transient
  `update_rejected` blip on the container's first S3 poll and checks
  eventual consistency instead (no active `error_state` on the listener,
  then a real functional request) - the container boots from an image
  whose baked-in config differs from what's pushed to S3, and that one-time
  transition can hit a real, still-open startup race between the fetcher's
  first overwrite and Envoy's own config load (root cause: `cds.yaml`/
  `lds.yaml` share one `watched_directory`, so writing one wakes Envoy's
  reload of the other before both files are mutually consistent - not yet
  fixed, tracked only here for now). Don't tighten this phase back to a
  zero-rejection check without fixing that first.

**If you edit this script**: it must never let `add-backend`/`remove-backend`
resolve a real `~/.config/gatewayctl/config.yaml` - it sets
`GATEWAYCTL_CONFIG` to a fresh temp path for exactly this reason, after an
earlier version of this script auto-pushed its test backends to this
project's own live production deployment on the first run. Don't remove
that without understanding why it's there. The same trap applies to any ad
hoc `gatewayctl` command run outside this script (e.g. while debugging a
failure by hand) - it happened again mid-session once already; always set
`GATEWAYCTL_CONFIG` to a scratch path first.

**Run from source, HTTP distribution mode** (two terminals - both run in
the foreground):
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

**gatewayctl dev loop:** self-contained Go module with its own
`go.mod`/`Makefile` under `gatewayctl/`; build/test/run it from there, not
from the repo root.

## Architecture

Clusters, routes, and fault-injection runtime state are **not** static in
`config/envoy.yaml`. Envoy's bootstrap only points
`dynamic_resources.cds_config`/`lds_config` at
`/etc/envoy/dynamic/{cds,lds}.yaml` with `watched_directory` set, and its
`layered_runtime` has a `disk_layer` pointed at
`/etc/envoy/dynamic/runtime/current` - so Envoy hot-reloads all three on
change, no xDS server, no gRPC streams.

```
gatewayctl (host)                                  Envoy container
  add-backend/remove-backend/fault               +-------------------------------+
        |                                         | config-fetcher (supervisor)  |
        v                                         |   polls HTTP or S3           |
  config/values.yaml -> render -> rendered/{cds,lds,runtime}.yaml | atomic-writes -v |
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

**Fault injection's disk_layer has a non-obvious constraint**: Envoy's
`layered_runtime.disk_layer` requires `symlink_root` to itself be a
symlink it watches for atomic replacement (the ConfigMap-volume trick),
**not** a plain directory whose files change in place - the latter was
tried first and silently never reloaded (confirmed via `/runtime` on the
admin API: the layer registered, `entries` stayed empty forever). The
fetcher writes each poll's runtime keys into a fresh `data-<digest>`
directory and swaps a `current` symlink to point at it, deleting the
previous one. See `docs/architecture.md` for the full writeup - read it
before touching `fetcher`'s `expandRuntimeLayer` or the `disk_layer`
config in `config/envoy.yaml`.

**Readiness is served by the fetcher, not Envoy**: Envoy's own `/ready`
goes live using whatever's baked into the image immediately at boot -
`dynamic_resources` are file-based, so there's always something local to
read - which is usually near-empty, since `rendered/` is gitignored and
real backend/fault state lives in the config server/S3, not the image.
Without a separate gate, a pod can be marked Ready by Kubernetes (and
start receiving traffic during a rolling deploy/scale-up) well before the
fetcher has ever synced the real config source, silently falling through
to `default_app` for anything it hasn't caught up on yet - a real,
observed gap, not hypothetical. The fetcher instead serves its own
`GET :8081/ready` (see `fetcher/main.go`; the Helm chart's
`readinessProbe` targets this port, not the admin one), which only
reports ready after the first full poll pass completes without a
fetch/write error (a legitimate "nothing pushed yet" counts as clean) and
never flips back - a pod that already converged once should stay in
rotation through a later transient poll failure, not drop out while still
serving good config. This is a deliberate fail-closed choice: a pod that
can't reach its config source at all stays NotReady indefinitely rather
than silently serving stale/seed config. It does **not** solve mixed
distribution-mode rollouts (e.g. v1 on `http`, v2 on `s3` mid-deploy) -
see the "Distribution modes" section below.

**gatewayctl internals** (`gatewayctl/internal/`):
- `config` - loads/saves `values.yaml` (`Backends` list, `Faults` map
  keyed by target name).
- `render` - builds Envoy CDS/LDS/runtime structs from `config.Values`
  (`BuildCDS`/`BuildLDS`/`BuildRuntime`) and marshals them to YAML
  (`Render`). Generated from typed Go structs, not text templates -
  avoids the whitespace/indentation bug class YAML templating is prone
  to, and a malformed field is a compile error instead of a runtime one.
- `envoyconfig` - the Go structs mirroring Envoy's CDS/LDS resource
  shape, plus `FaultRuntimeKeys`/`FaultPerRoute` (the runtime-key naming
  convention shared between rendering and the `fault` commands).
- `settings` - gatewayctl's own settings file (`~/.config/gatewayctl/config.yaml`
  by default, overridable via `--config`/`GATEWAYCTL_CONFIG`): distribution
  mode (`http`/`s3`) plus the config server URL/token or S3 bucket/prefix.
  Set once via `gatewayctl config set ...` so `add-backend`/`remove-backend`/
  `fault` push automatically afterward.

**gatewayctl commands beyond add/remove-backend and fault** (`gatewayctl/cmd/`):
- `list-backends [--remote]` - local reads `values.yaml`; `--remote`
  fetches and reconstructs the same table from the config server's/S3's
  actual `cds.yaml`/`lds.yaml`, since local state can drift (a push that
  failed silently, or running from a different machine/directory than
  whoever last pushed).
- `fetch` - downloads the raw `cds.yaml`/`lds.yaml`/`runtime.yaml` into
  `--rendered-dir`.
- `generate-values` - reconstructs a full `values.yaml` from what's live
  (`kubectl get -o yaml` style). Every field round-trips except a
  no-route backend's `--timeout`, which is a route-level Envoy property
  never persisted anywhere when there's no route. Refuses to overwrite an
  existing `values.yaml` unless `--force`.
- `remote.go` holds `findRemoteRoute`/`fetchRemoteCDSAndLDS`, shared by
  `list-backends --remote` and `generate-values` - don't duplicate that
  matching logic if extending either.

**Safety checks worth knowing about before changing `add_backend.go`/`fault.go`:**
- `--name`/`--target` are validated against `[a-zA-Z0-9_-]+`
  (`validateName`) - both become Envoy runtime keys the fetcher turns
  into filesystem paths, so an unvalidated `../../etc/cron.d/evil` would
  be a path-traversal write on the gateway (previously a real,
  fixed vulnerability - see the `expandRuntimeLayer` guard in `fetcher/main.go`
  for the defense-in-depth side of the same fix).
- `--port`, `--percent`, `--status`, `--duration-ms`,
  `--connect-timeout`/`--timeout` are all range/format-validated
  (`validatePort`/`validatePercent`/`validateHTTPStatus`/`validateNonNegative`/
  `validateDuration` in `cmd/root.go`) before anything is saved or pushed.
- `add-backend` refuses to run if local `values.yaml` doesn't exist but a
  mode is configured, unless `--force` - guards the wrong-directory/
  wrong-machine trap where a missing file looks identical to a legitimate
  first backend but would silently overwrite a remote config that already
  has other backends. `remove-backend` doesn't need this: it already
  fails safe (errors on "no backend named X" before ever saving/pushing).

**Routing model** (see `render.BuildCDS`/`BuildLDS`):
- `--route-prefix` routes that path prefix to the backend, rewritten to
  `/` upstream. Omit it to register a cluster with no route.
- `--domain` gives a backend its own Envoy virtual host matched on the
  `Host` header, instead of sharing the catch-all one. Combine with
  `--route-prefix` to scope it to a path under that domain.
- `--host-header` sets `host_rewrite_literal` on the route - the Host
  header the *upstream* sees, independent of `--host`/TLS SNI (which only
  control the connection). Needed for any backend that validates Host
  against its own domain (Cloudflare-fronted ones, e.g.), or it rejects
  with `421 Misdirected Request`.
- Path-prefix routing only works cleanly for self-contained responses
  (APIs); a real frontend's absolute asset paths (`/assets/*.js`) miss
  the prefix and fall through to `default_app` - use `--domain` for those
  instead. See the User Guide's Routing section for the full writeup,
  including the CORS/cookie-domain caveats of proxying a third-party
  origin under a prefix.
- Backends with neither `--domain` nor `--route-prefix` still get a
  cluster, just no route.
- Unmatched requests fall through to `default_app` (`:5050`).
- `--http2` sets `typed_extension_protocol_options` on the cluster so
  Envoy speaks HTTP/2 to that upstream (plus ALPN `h2` if `--tls` too) -
  required for gRPC backends, since Envoy otherwise defaults every
  cluster to HTTP/1.1 regardless of what the listener/client negotiated.
  Use `--domain` with it, not `--route-prefix` (path rewriting breaks
  gRPC's fixed paths). A gRPC/HTTP2 cluster needs its own struct fields
  (`envoyconfig.Cluster.TypedExtensionProtocolOptions`,
  `UpstreamTLSContext.AlpnProtocols`) - there was no way to express this
  before this flag existed.
- `--compression gzip` enables gzip response compression for one
  backend's route, off by default and opt-in per backend (not
  gateway-wide - compression would otherwise silently skew a perf test).
  Implemented as a listener-wide `envoy.filters.http.compressor` filter
  registered disabled (`render.go`'s `HTTPFilters`), with a per-route
  `typed_per_filter_config` override turning it on - same "global default
  + per-route override" shape `FaultPerRoute` already uses. **Non-obvious
  gotcha**: the per-route override is a *different* Envoy message
  (`CompressorPerRoute`) from the filter's own top-level config
  (`Compressor`) - reusing the latter for both, as an earlier version of
  this code did, fails at listener-load time with "Unable to unpack as
  ...CompressorPerRoute", only visible via the admin API's
  `/config_dump?resource=dynamic_listeners` `error_state` field, not a Go
  compile error. Confirmed the fix against a real Envoy instance, not
  just docs - if you add another per-route-overridable filter, check its
  proto for a similar dedicated `*PerRoute` message before assuming the
  filter's own config type works for both places.

**Fault injection**: every route (each backend plus `default_app`) gets an
independently toggleable fault config via a unique Envoy runtime key per
`--target` (`gatewayctl fault abort|delay|reset --target <name>`), so
faulting one target never affects another's traffic. Fault state is
persisted through the same config-distribution pipeline as
backends/routes (`values.yaml`'s `faults` map -> `runtime.yaml` ->
`disk_layer`), not just a live admin-API POST - see the disk_layer note
above for why that distinction matters (every gateway replica converges,
and it survives restarts). `layered_runtime.admin` still sits above the
disk layer for an instant, single-pod, in-memory-only override via a
direct `POST /runtime_modify`.

**Distribution modes**, selected by `CONFIG_SOURCE_KIND`:
- `http` - `fetcher` polls `config_server` (its own Go module, own
  `storage/` dir, deployable standalone, or via the
  `envoy-perf-gateway-config-server` Helm chart); `gatewayctl push-http`
  POSTs rendered YAML to it. Gated by `API_TOKEN`. Cloud-agnostic.
- `s3` - `fetcher` polls an S3 bucket; `gatewayctl push-s3` uploads
  `rendered/` there. AWS-specific (or an S3-compatible API). Useful once
  config needs to be shared across multiple gateway instances.

See `docs/architecture.md` for the full picture with diagram, and
`docs/index.html` (the published User Guide) for user-facing CLI/routing/
Helm documentation.
