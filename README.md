# envoy-perf-gateway

Envoy as a front door for perf testing. Add and remove backends with a CLI,
no restart, no full xDS control plane.

## Install

```bash
make venv && source .venv/bin/activate   # or: pip install -e "./gatewayctl[s3]"
gatewayctl --help
```

`gatewayctl/` is a self-contained sub-project - its own `pyproject.toml`,
own `Makefile`, own dependencies, `templates/*.j2` bundled as package data -
structured as if it were a separate repo, even though it lives inside this
one. That's what makes it produce a wheel installable anywhere, not just
editable from a checkout of this repo (see below). `values.yaml` and the
`rendered/` output directory still default to the current working
directory and can be pointed elsewhere with `--values`/`--rendered-dir`
(or `GATEWAYCTL_VALUES`/`GATEWAYCTL_RENDERED_DIR`), e.g. to run against
multiple checkouts or a values file living outside any repo.

### Building a standalone gatewayctl wheel

```bash
make build-gatewayctl   # or: cd gatewayctl && make build
pip install gatewayctl/dist/*.whl
```

See [docs/architecture.md](docs/architecture.md) for how config flows from
`gatewayctl` through to a running Envoy, and why the HTTP/S3 distribution
split exists.

## Quickstart (HTTP source)

```bash
make venv && source .venv/bin/activate
make all                       # build image
make -C config_server up       # build + run config_server container, :8090
make run                       # run gateway container, polling it over HTTP

gatewayctl push-http --server-url http://localhost:8090
curl http://localhost:8080/
```

### Authenticating the config server

Set `API_TOKEN` before starting `config_server` to require it on every
`/config/*` request (`/healthz` stays open for liveness probes):

```bash
export API_TOKEN=some-long-random-value
make -C config_server up

gatewayctl push-http --server-url http://localhost:8090 --api-token $API_TOKEN
# or: export CONFIG_SERVER_API_TOKEN=$API_TOKEN and drop --api-token
```

The gateway container's fetcher needs the same token to download, via
`CONFIG_API_TOKEN`:

```bash
export CONFIG_API_TOKEN=$API_TOKEN
make run   # picks up CONFIG_API_TOKEN from the shell env
```

## Quickstart (S3 source)

```bash
make venv && source .venv/bin/activate
make all
export CONFIG_S3_BUCKET=my-bucket CONFIG_S3_PREFIX=envoy-perf-gateway/
make run-s3   # needs AWS credentials in your shell env (AWS_ACCESS_KEY_ID etc.)
```

`add-backend`/`remove-backend` render locally either way; in S3 mode you
also need `gatewayctl push-s3 --bucket my-bucket --prefix envoy-perf-gateway/`
after each change (or export `CONFIG_S3_BUCKET`/`CONFIG_S3_PREFIX` so the
flag can be omitted) for the fetcher to pick it up.

Enable S3 bucket versioning so pushes are rollback-able via
`aws s3api list-object-versions` / `restore-object`. `values.yaml` is the
human-authored source of truth and is a good candidate to `git commit`
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

`--route-prefix` is optional - omit it to register the cluster without
wiring a route (e.g. if you'll reference it from a hand-edited route later).
Requests are matched with a path prefix and rewritten to `/` on the
upstream. Everything not matched by a backend route falls through to the
`default_app` cluster (the bundled Flask echo server on :5050).

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
