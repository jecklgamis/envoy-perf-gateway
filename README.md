# envoy-perf-gateway

Envoy as a front door for perf testing. Add and remove backends with a CLI,
no restart, no full xDS control plane.

## Install

```bash
make -C gatewayctl install   # go install - puts gatewayctl on your $PATH
gatewayctl --help
```

`gatewayctl` is a Go CLI - a static binary with no runtime dependency, so
it's easy to hand to teammates who don't have Go (or anything else)
installed at all. `gatewayctl/` is a self-contained module (own `go.mod`, own `Makefile`),
structured as if it were a separate repo even though it lives inside this
one. `config/values.yaml` and the `rendered/` output directory default to
paths relative to the current working directory and can be pointed
elsewhere with `--values`/`--rendered-dir` (or
`GATEWAYCTL_VALUES`/`GATEWAYCTL_RENDERED_DIR`), e.g. to run against
multiple checkouts or a values file living outside any repo.

### Building gatewayctl for other platforms

```bash
make build-gatewayctl-all   # or: cd gatewayctl && make build-all
ls gatewayctl/dist/          # gatewayctl-{darwin,linux}-{arm64,amd64}
```

Each is a standalone binary - no install step needed, just copy it
somewhere on `$PATH` and run it.

See [docs/architecture.md](docs/architecture.md) for how config flows from
`gatewayctl` through to a running Envoy, and why the HTTP/S3 distribution
split exists.

## Quickstart (HTTP source)

`config_server` always requires `API_TOKEN` - it refuses to start without
one, there's no unauthenticated mode. `config_server/Makefile`'s `run`/`up`
default it to `default` for local dev convenience if you don't export
anything - fine on localhost, but export a real value for anything beyond
that:

```bash
export API_TOKEN=some-long-random-value   # or skip this and use "default" locally

make -C gatewayctl install
make all                       # build image
make -C config_server up       # build + run config_server container, :8090

# the gateway's fetcher needs the same token to download, via CONFIG_API_TOKEN
export CONFIG_API_TOKEN=$API_TOKEN        # or "default" to match the skipped case above
make run                       # run gateway container, polling it over HTTP

gatewayctl push-http --server-url http://localhost:8090 --api-token $API_TOKEN
# or: export CONFIG_SERVER_API_TOKEN=$API_TOKEN and drop --api-token
# or: gatewayctl config set http.api-token $API_TOKEN (persists across sessions)
curl http://localhost:8080/
```

## Quickstart (S3 source)

```bash
make -C gatewayctl install
make all
export CONFIG_S3_BUCKET=my-bucket CONFIG_S3_PREFIX=envoy-perf-gateway/
make run-s3   # needs AWS credentials in your shell env (AWS_ACCESS_KEY_ID etc.)
```

`add-backend`/`remove-backend` render locally either way; in S3 mode you
also need `gatewayctl push-s3 --bucket my-bucket --prefix envoy-perf-gateway/`
after each change (or export `CONFIG_S3_BUCKET`/`CONFIG_S3_PREFIX` so the
flag can be omitted) for the fetcher to pick it up.

Enable S3 bucket versioning so pushes are rollback-able via
`aws s3api list-object-versions` / `restore-object`. `config/values.yaml`
is the human-authored source of truth and is a good candidate to `git commit`
before each render/push, so backend changes have real history on both
sides.

## Adding a backend to test

```bash
gatewayctl add-backend \
  --name httpbin --host httpbin.org --port 443 --tls \
  --route-prefix /httpbin/

# push it to wherever the fetcher is polling - HTTP or S3, pick one
gatewayctl push-http --server-url http://localhost:8090
gatewayctl push-s3 --bucket my-bucket --prefix envoy-perf-gateway/

curl http://localhost:8080/httpbin/get

gatewayctl list-backends
gatewayctl remove-backend --name httpbin
# ...and push again to make the removal take effect
```

### Skipping the separate push step

Set a mode and `add-backend`/`remove-backend` push automatically after
every change - no separate `push-http`/`push-s3` call needed. Two ways to
set it:

**Settings file** (persists across sessions, `~/.config/gatewayctl/config.yaml`
by default - override with `--config`/`GATEWAYCTL_CONFIG`):

```bash
gatewayctl config set mode http
gatewayctl config set http.server-url http://localhost:8090
gatewayctl config set http.api-token some-token   # if config_server requires one

# or for S3:
gatewayctl config set mode s3
gatewayctl config set s3.bucket my-bucket
gatewayctl config set s3.prefix envoy-perf-gateway/

gatewayctl config get              # see everything configured
gatewayctl config get mode         # or just one value
gatewayctl config set mode ""   # back to no auto-push
```

**Env vars** (session-scoped, useful in CI or to override the settings
file for one shell):

```bash
export CONFIG_SOURCE_KIND=http
export CONFIG_SERVER_URL=http://localhost:8090       # + CONFIG_SERVER_API_TOKEN if set

# or for S3:
export CONFIG_SOURCE_KIND=s3
export CONFIG_S3_BUCKET=my-bucket CONFIG_S3_PREFIX=envoy-perf-gateway/
```

```bash
gatewayctl add-backend --name httpbin --host httpbin.org --port 443 --tls --route-prefix /httpbin/
# already pushed - no push-http/push-s3 needed
```

Precedence per value is env var > settings file > built-in default. Leave
both unset and nothing changes - `add-backend`/`remove-backend` only touch
`config/values.yaml` and `rendered/`, same as before. The settings file is
written with `0600` permissions since `http.api-token` may hold a secret.

`--route-prefix` is optional - omit it to register the cluster without
wiring a route (e.g. if you'll reference it from a hand-edited route later).
Requests are matched with a path prefix and rewritten to `/` on the
upstream. Everything not matched by a backend route falls through to the
`default_app` cluster (the bundled Go echo server on :5050).

### Frontend/backend pairs (domain-based routing)

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

Run a load test (e.g. [fortio](https://github.com/fortio/fortio)) against
`http://localhost:8080/` to characterize the gateway's overhead, or against
a backend added via `add-backend` to test it through a realistic front
door.
