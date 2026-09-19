# envoy-perf-gateway

[![Build Gateway](https://github.com/jecklgamis/envoy-perf-gateway/actions/workflows/build-gateway.yaml/badge.svg)](https://github.com/jecklgamis/envoy-perf-gateway/actions/workflows/build-gateway.yaml)
[![Build Config Server](https://github.com/jecklgamis/envoy-perf-gateway/actions/workflows/build-config-server.yaml/badge.svg)](https://github.com/jecklgamis/envoy-perf-gateway/actions/workflows/build-config-server.yaml)

Envoy as a front door for perf and chaos testing. Add and remove backends
with a CLI, toggle fault injection at runtime, no restart, no full xDS
control plane.

## Features

- **Per-backend fault isolation** - inject aborts and delays into one
  backend's traffic without touching any other route. No blast radius,
  no shared kill switch.
- **Live toggling, zero restarts** - flip fault injection on and off
  against a running gateway via Envoy's admin API. No redeploy, no
  config reload, no dropped connections.
- **No xDS control plane to run** - dynamic backends via filesystem
  CDS/LDS and inotify hot-reload. Skip the usual xDS server, gRPC
  streams, and cluster bootstrap ceremony.
- **One CLI for the whole workflow** - the Gateway CLI adds/removes
  backends, pushes config, and drives fault injection, all from one
  binary with no YAML hand-editing.
- **Pluggable config source** - push backend config over HTTP or
  straight to S3, pick whichever fits your test environment.
- **Ready in one command** - pre-built Docker images and CLI binaries
  published on every release, nothing to build to get started.

## Quickstart

Nothing to build - pulls the published Docker images and a pre-built
Gateway CLI (`gatewayctl`) binary from the
[releases page](https://github.com/jecklgamis/envoy-perf-gateway/releases).

**1. Run the gateway, already pointed at the config server** (fine if
the config server isn't up yet - it serves a baked-in default in the
meantime). In its own terminal, foreground on purpose - use a new
terminal for each step from here on:

```bash
docker pull jecklgamis/envoy-perf-gateway:latest
docker run --name envoy-perf-gateway -p 8080:8080 -p 9901:9901 \
  -e CONFIG_SOURCE_KIND=http \
  -e CONFIG_SOURCE_URL=http://host.docker.internal:8090 \
  -e CONFIG_API_TOKEN=default \
  jecklgamis/envoy-perf-gateway:latest
```

**2. In another terminal, bring up the config server:**

```bash
docker pull jecklgamis/envoy-perf-gateway-config-server:latest
docker run --name envoy-perf-gateway-config-server -p 8090:8090 \
  -e API_TOKEN=default \
  jecklgamis/envoy-perf-gateway-config-server:latest
```

**3. In a third terminal, download the Gateway CLI (`gatewayctl`)**
(pick your platform -
check the releases page for the current tag; GitHub's `releases/latest`
link only resolves once a non-prerelease version is published):

```bash
curl -L -o gatewayctl https://github.com/jecklgamis/envoy-perf-gateway/releases/download/v1.0.0-alpha.1/gatewayctl-darwin-arm64
chmod +x gatewayctl
```

Other platforms: swap the suffix for `gatewayctl-darwin-amd64`,
`gatewayctl-linux-amd64`, or `gatewayctl-linux-arm64`.

**4. Point the Gateway CLI at the config server** - this saves the mode, URL, and
token to the Gateway CLI's settings file (`~/.config/gatewayctl/config.yaml`
by default) so `add-backend`/`remove-backend` push automatically from here
on:

```bash
./gatewayctl config set mode http
./gatewayctl config set http.server-url http://localhost:8090
./gatewayctl config set http.api-token default
```

**5. Add a real backend** - no restart of the gateway needed, it's already
polling the config server, this is where the Gateway CLI earns its keep,
dynamically wiring in a backend:

```bash
./gatewayctl add-backend --name httpbin --host httpbin.org --port 443 --tls --route-prefix /httpbin/

curl http://localhost:8080/httpbin/get
```

**6. Try fault injection against it** - isolated per backend, so this only
affects `httpbin` traffic, nothing else:

```bash
./gatewayctl fault abort --target httpbin --percent 100 --status 503
curl http://localhost:8080/httpbin/get     # now 503
curl http://localhost:8080/                # unaffected - still default_app

./gatewayctl fault delay --target httpbin --percent 100 --duration-ms 2000
curl http://localhost:8080/httpbin/get     # now takes ~2s

./gatewayctl fault reset --target httpbin
curl http://localhost:8080/httpbin/get     # back to normal
```

## Building

**Gateway:**

```bash
make all   # builds the envoy-perf-gateway image
```

**Config server:**

```bash
make -C config_server image
```

**Gateway CLI:**

```bash
make -C gatewayctl install     # go install, puts it on $PATH
make build-gatewayctl-all      # cross-compile all platforms into gatewayctl/dist/
```

See [docs/architecture.md](docs/architecture.md) for how config flows from
the Gateway CLI through to a running Envoy, and why the HTTP/S3 distribution
split exists.

## Installing The CLI

Download a pre-built binary from the
[releases page](https://github.com/jecklgamis/envoy-perf-gateway/releases)
(see Quickstart step 3), or install with Go:

```bash
go install github.com/jecklgamis/envoy-perf-gateway/gatewayctl@latest
```

## Running From Source (HTTP Source)

```bash
make -C config_server up
make up
```

Both default `API_TOKEN`/`CONFIG_API_TOKEN` to `default` if you don't
export your own - fine on localhost, export a real value for anything
beyond that.

## Running From Source (S3 Source)

```bash
make -C gatewayctl install
make all
export CONFIG_S3_BUCKET=my-bucket CONFIG_S3_PREFIX=envoy-perf-gateway/
make run-s3   # needs AWS credentials in your shell env (AWS_ACCESS_KEY_ID etc.)
```

Configure the Gateway CLI once (`gatewayctl config set mode s3`, `s3.bucket`,
`s3.prefix` - same idea as Quickstart step 4) so `add-backend`/`remove-backend`
push to S3 automatically.

## Managing Backends

`gatewayctl list-backends` and `gatewayctl remove-backend --name <name>`
round out `add-backend` from the Quickstart.

```bash
gatewayctl list-backends
# httpbin              httpbin.org:443  tls        /httpbin/
# svc-a                svc-a.internal:8080  plaintext  frontend-a.test.local

gatewayctl remove-backend --name httpbin
```

`list-backends` reads straight from `values.yaml` and prints one line per
backend - name, upstream host:port, TLS mode, and the resolved route (domain
and/or path prefix, or `(no route, cluster only)` if neither was set).
`remove-backend` drops the named backend's cluster and route, then pushes
the updated config the same way `add-backend` does.

### Path Based Routing

`--route-prefix` on `add-backend` routes requests under that path prefix to
the backend, rewritten to `/` on the upstream:

```bash
gatewayctl add-backend --name httpbin --host httpbin.org --port 443 --tls --route-prefix /httpbin/

curl http://localhost:8080/httpbin/get
```

It's optional - omit it to register the cluster without a route. Unmatched
requests fall through to `default_app` (the bundled echo server on :5050).

### Virtual Host Routing

Pair a backend with a specific frontend `Host` header instead of (or in
addition to) a path prefix, via `--domain`. Each domain gets its own Envoy
virtual host, matched by the request's `Host` header rather than sharing the
catch-all one:

```bash
gatewayctl add-backend \
  --name svc-a --host svc-a.internal --port 8080 \
  --domain frontend-a.test.local

gatewayctl add-backend \
  --name svc-b --host svc-b.internal --port 8080 \
  --domain frontend-b.test.local --route-prefix /api/

curl -H "Host: frontend-a.test.local" http://localhost:8080/
curl -H "Host: frontend-b.test.local" http://localhost:8080/api/anything
```

`--domain` alone routes everything under that Host header to the backend
(`/` rewritten to nothing). Combined with `--route-prefix`, only that path
prefix under the domain is routed there (rewritten to `/`), same as the
path-only case. Backends with neither `--domain` nor `--route-prefix` set
still register a cluster with no route at all.

## Fault Injection

Every route - each `add-backend`, plus the `default_app` fallback - gets
its own independently-toggleable fault injection, isolated via a unique
Envoy runtime key per `--target`. Faulting one backend never affects any
other's traffic. Toggled live via the Envoy admin API - no config reload
needed, takes effect on the next request:

```bash
# 30% of backend-1's requests get a 503 - backend-2, default_app, etc. untouched
gatewayctl fault abort --target backend-1 --percent 30 --status 503

# 20% of backend-1's requests get a 2s delay
gatewayctl fault delay --target backend-1 --percent 20 --duration-ms 2000

# back to baseline for backend-1
gatewayctl fault reset --target backend-1

# target the default_app fallback route instead
gatewayctl fault abort --target default_app --percent 100 --status 503
```

`--target` is the backend name exactly as passed to `add-backend --name`,
or `default_app` for the fallback route. This requires the
`layered_runtime.admin` layer in `config/envoy.yaml` - without it,
`/runtime_modify` returns `503 No admin layer specified`.

Run a load test (e.g. [fortio](https://github.com/fortio/fortio)) against
`http://localhost:8080/` while toggling these to see how your client-side
retry/timeout/circuit-breaker behavior holds up under a degraded upstream,
or just against a backend added via `add-backend` to characterize it
through a realistic front door.
