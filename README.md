# envoy-perf-gateway

[![Build Gateway](https://github.com/jecklgamis/envoy-perf-gateway/actions/workflows/build-gateway.yaml/badge.svg)](https://github.com/jecklgamis/envoy-perf-gateway/actions/workflows/build-gateway.yaml)
[![Build Config Server](https://github.com/jecklgamis/envoy-perf-gateway/actions/workflows/build-config-server.yaml/badge.svg)](https://github.com/jecklgamis/envoy-perf-gateway/actions/workflows/build-config-server.yaml)

Envoy as a front door for performance and chaos testing. Add and remove
backends through a CLI, and toggle fault injection at runtime, with no
restarts and no full xDS control plane.

## Features

- **Per-backend fault isolation.** Inject aborts and delays into a single
  backend without affecting any other route.
- **Live toggling, zero restarts.** Fault injection is controlled through
  Envoy's admin API, with no redeploy or config reload required.
- **No xDS control plane required.** Backends are updated dynamically via
  filesystem-based CDS/LDS and inotify hot-reload.
- **A single CLI for the workflow.** Add and remove backends, push
  configuration, and control fault injection, all without hand-editing
  YAML.
- **Pluggable configuration source.** Distribute configuration over HTTP
  or directly to S3.
- **Ready to run immediately.** Pre-built Docker images and CLI binaries
  are published with every release.

## Quickstart

This uses the published Docker images and a pre-built `gatewayctl` binary
from the [releases page](https://github.com/jecklgamis/envoy-perf-gateway/releases);
no build step is required.

**1. Start the gateway** (in its own terminal). This works even if the
config server is not yet running, since it serves a built-in default
configuration in the meantime:

```bash
docker pull jecklgamis/envoy-perf-gateway:latest
docker run --name envoy-perf-gateway -p 8080:8080 -p 9901:9901 \
  -e CONFIG_SOURCE_KIND=http \
  -e CONFIG_SOURCE_URL=http://host.docker.internal:8090 \
  -e CONFIG_API_TOKEN=default \
  jecklgamis/envoy-perf-gateway:latest
```

**2. Start the config server** (in a separate terminal):

```bash
docker pull jecklgamis/envoy-perf-gateway-config-server:latest
docker run --name envoy-perf-gateway-config-server -p 8090:8090 \
  -e API_TOKEN=default \
  jecklgamis/envoy-perf-gateway-config-server:latest
```

**3. Download `gatewayctl`** for your platform. Check the
[releases page](https://github.com/jecklgamis/envoy-perf-gateway/releases)
for the current tag:

```bash
curl -L -o gatewayctl https://github.com/jecklgamis/envoy-perf-gateway/releases/download/v1.0.0-alpha.1/gatewayctl-darwin-arm64
chmod +x gatewayctl
```

Other platforms: `gatewayctl-darwin-amd64`, `gatewayctl-linux-amd64`,
`gatewayctl-linux-arm64`.

Alternatively, install it with Go:

```bash
go install github.com/jecklgamis/envoy-perf-gateway/gatewayctl@latest
```

**4. Point the CLI at the config server.** This is saved to
`~/.config/gatewayctl/config.yaml`, so subsequent `add-backend` and
`remove-backend` commands push configuration automatically:

```bash
./gatewayctl config set mode http
./gatewayctl config set http.server-url http://localhost:8090
./gatewayctl config set http.api-token default
```

**5. Add a backend.** No gateway restart is required:

```bash
./gatewayctl add-backend --name httpbin --host httpbin.org --port 443 --tls --route-prefix /httpbin/

curl http://localhost:8080/httpbin/get
```

**6. Test fault injection**, scoped to the `httpbin` backend:

```bash
./gatewayctl fault abort --target httpbin --percent 100 --status 503
curl http://localhost:8080/httpbin/get     # returns 503
curl http://localhost:8080/                # unaffected - still default_app

./gatewayctl fault delay --target httpbin --percent 100 --duration-ms 2000
curl http://localhost:8080/httpbin/get     # now takes ~2s

./gatewayctl fault reset --target httpbin
curl http://localhost:8080/httpbin/get     # back to normal
```

## Building

```bash
make all                        # gateway image
make -C config_server image     # config server image
make -C gatewayctl install      # install gatewayctl to $PATH
make build-gatewayctl-all       # cross-compile all platforms to gatewayctl/dist/
```

See [docs/architecture.md](docs/architecture.md) for how configuration
flows from the Gateway CLI through to a running Envoy instance, and the
rationale for the HTTP/S3 distribution split.

## Installing The CLI

Download a pre-built binary from the
[releases page](https://github.com/jecklgamis/envoy-perf-gateway/releases),
or install it with Go:

```bash
go install github.com/jecklgamis/envoy-perf-gateway/gatewayctl@latest
```

## Running From Source

**HTTP source:**

```bash
make -C config_server up
make up
```

**S3 source:**

```bash
make -C gatewayctl install
make all
export CONFIG_S3_BUCKET=my-bucket CONFIG_S3_PREFIX=envoy-perf-gateway/
make run-s3   # requires AWS credentials in the shell environment (AWS_ACCESS_KEY_ID, etc.)
```

Configure the CLI once (`gatewayctl config set mode s3`, `s3.bucket`,
`s3.prefix`) so that `add-backend` and `remove-backend` push to S3
automatically. `API_TOKEN` and `CONFIG_API_TOKEN` both default to
`default` if unset; this is acceptable for local development only. Export
a real value for any other environment.

## Managing Backends

`gatewayctl list-backends` and `gatewayctl remove-backend --name <name>`
complement `add-backend`:

```bash
gatewayctl list-backends
# httpbin              httpbin.org:443  tls        /httpbin/
# svc-a                svc-a.internal:8080  plaintext  frontend-a.test.local

gatewayctl remove-backend --name httpbin
```

`list-backends` prints the name, upstream host:port, TLS mode, and
resolved route for each backend (or `(no route, cluster only)` if none is
configured). `remove-backend` removes the backend's cluster and route,
then pushes the updated configuration, the same way `add-backend` does.

### Path Based Routing

`--route-prefix` routes requests under that path prefix to the backend,
rewritten to `/` on the upstream. This flag is optional; omitting it
registers the cluster without a route. Unmatched requests fall through to
`default_app`, the bundled echo server on port `5050`.

### Virtual Host Routing

`--domain` gives a backend its own Envoy virtual host, matched on the
`Host` header, instead of or in addition to a path prefix:

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

`--domain` alone routes all traffic under that Host header to the backend.
Combined with `--route-prefix`, only that path prefix under the domain is
routed. If neither flag is set, the backend still registers a cluster
without a route.

## Fault Injection

Each route, including the `default_app` fallback, has its own
independently toggleable fault injection, isolated by a unique Envoy
runtime key per `--target`. Faulting one backend does not affect any
other's traffic. Changes are applied live through the Envoy admin API,
with no config reload, and take effect on the next request:

```bash
# 30% of backend-1's requests return 503 - backend-2, default_app, etc. are unaffected
gatewayctl fault abort --target backend-1 --percent 30 --status 503

# 20% of backend-1's requests are delayed by 2s
gatewayctl fault delay --target backend-1 --percent 20 --duration-ms 2000

# reset backend-1 to baseline
gatewayctl fault reset --target backend-1

# target the default_app fallback route instead
gatewayctl fault abort --target default_app --percent 100 --status 503
```

`--target` is the backend's `--name` value, or `default_app` for the
fallback route. This requires the `layered_runtime.admin` layer in
`config/envoy.yaml`; without it, `/runtime_modify` returns
`503 No admin layer specified`.

Run a load test (for example, [fortio](https://github.com/fortio/fortio))
against `http://localhost:8080/` while toggling these settings to observe
how client-side retry, timeout, and circuit-breaker behavior holds up
under a degraded upstream.
