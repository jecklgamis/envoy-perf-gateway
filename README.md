# envoy-perf-gateway

[![Build Gateway](https://github.com/jecklgamis/envoy-perf-gateway/actions/workflows/build-gateway.yaml/badge.svg)](https://github.com/jecklgamis/envoy-perf-gateway/actions/workflows/build-gateway.yaml)
[![Build Config Server](https://github.com/jecklgamis/envoy-perf-gateway/actions/workflows/build-config-server.yaml/badge.svg)](https://github.com/jecklgamis/envoy-perf-gateway/actions/workflows/build-config-server.yaml)

Envoy as a front door for performance and chaos testing. Add and remove
backends through a CLI, and toggle fault injection at runtime, with no
restarts and no full xDS control plane.

**[User Guide](https://jecklgamis.github.io/envoy-perf-gateway/)** - full
docs on managing backends, routing, timeouts, fault injection, perf
testing, running from source, and Kubernetes/Helm deployment.

## Features

- **Per-backend fault isolation.** Inject aborts and delays into a single
  backend without affecting any other route.
- **Live toggling, zero restarts.** Fault injection distributes like a
  backend change - no redeploy - and converges across every gateway
  replica, not just whichever one an ad-hoc call happens to reach.
- **No xDS control plane required.** Backends, routes, and fault injection
  are all updated dynamically via filesystem-based CDS/LDS/runtime and
  inotify hot-reload.
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

**6. Test fault injection**, scoped to the `httpbin` backend. Like
`add-backend`, this pushes through the config server, so allow up to
`CONFIG_POLL_INTERVAL_SECONDS` (15s by default) for the gateway to pick it
up:

```bash
./gatewayctl fault abort --target httpbin --percent 100 --status 503
curl http://localhost:8080/httpbin/get     # returns 503 within ~15s
curl http://localhost:8080/                # unaffected - still default_app

./gatewayctl fault delay --target httpbin --percent 100 --duration-ms 2000
curl http://localhost:8080/httpbin/get     # now takes ~2s

./gatewayctl fault reset --target httpbin
curl http://localhost:8080/httpbin/get     # back to normal
```

From here, see the **[User Guide](https://jecklgamis.github.io/envoy-perf-gateway/)**
for path/virtual-host routing, timeouts, how fault injection is
distributed and made to survive restarts/scale across replicas, perf
testing tool examples, running from source, and Kubernetes/Helm
deployment.

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

## Contributing

Found a bug, have a feature request, or want to submit a fix? Open an
[issue](https://github.com/jecklgamis/envoy-perf-gateway/issues) or a
[pull request](https://github.com/jecklgamis/envoy-perf-gateway/pulls).

## License

[Apache License 2.0](LICENSE)
