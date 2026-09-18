# envoy-perf-gateway

Envoy as a front door for perf testing. Add and remove backends with a CLI,
no restart, no full xDS control plane.

## Install

```bash
make venv && source .venv/bin/activate   # or: pip install -e ".[s3]"
gatewayctl --help
```

`gatewayctl` is a proper installed console script (`gatewayctl/` is a real
Python package, see `pyproject.toml`), not a script you invoke with
`python3 path/to/file.py`. `templates/` is always resolved relative to this
repo, but `values.yaml` and the `rendered/` output directory default to
this repo's paths and can be pointed elsewhere with `--values`/
`--rendered-dir` (or `GATEWAYCTL_VALUES`/`GATEWAYCTL_RENDERED_DIR`), e.g. to
run against multiple checkouts or a values file living outside the repo.

## Architecture

```
gatewayctl (host)                                  Envoy container
  add-backend/remove-backend                     +-------------------------------+
        |                                         | config-fetcher (supervisor)  |
        v                                         |   polls HTTP or S3           |
  values.yaml -> render -> rendered/{cds,lds}.yaml |   atomic-writes into --v      |
        |                                         v                          |   |
        |                              /etc/envoy/dynamic  <------------------+
        |  HTTP: gatewayctl push-http uploads to config_server's own storage/       |
        |  S3:   gatewayctl push-s3 uploads rendered/ to a bucket                  v
        +------------------------------------------------------> Envoy inotify watch -> hot-reload
```

Clusters and routes are **not** static in `config/envoy.yaml`. The bootstrap
only points `dynamic_resources.cds_config` / `lds_config` at
`/etc/envoy/dynamic/{cds,lds}.yaml` inside the container, with
`watched_directory` set so Envoy watches that directory via inotify.

`config/dynamic` is **not** bind-mounted from the host. On Docker Desktop
for Mac, host-side writes into a bind-mounted directory sync file content
into the container but do not reliably propagate the underlying inotify
event, so Envoy never notices the change even though the file is correct.
Instead, `fetcher/config_fetcher.py` runs *inside* the container
(via `supervisor.ini`) and polls a remote source for `cds.yaml`/`lds.yaml`,
writing them into `/etc/envoy/dynamic` itself - a write native to the
container's own filesystem, which does trigger Envoy's inotify watch.
`gatewayctl` on the host only ever writes to `values.yaml` and the local
`rendered/` directory; it never touches the container's filesystem
directly. Both `gatewayctl` and the fetcher write atomically (temp file +
`os.replace`, not delete-then-write) so a reader never observes a missing
or partial file mid-swap.

Two distribution mechanisms are supported, selected by the
`CONFIG_SOURCE_KIND` env var on the container. Both are cloud-agnostic
(no dependency on a specific provider) and both require an explicit push
after each change - `gatewayctl` never touches the container or the fetcher's
source directly, only the distribution endpoint:

- **`http`** - `config_fetcher.py` polls `config_server/config_server.py`,
  a small Flask app with its own `storage/` directory (has its own
  `Dockerfile`/`Makefile` - deployable as its own service). `gatewayctl
  push-http` uploads `rendered/cds.yaml`/`lds.yaml` to it over HTTP POST.
  The server doesn't need to be colocated with `gatewayctl` - anywhere
  reachable over HTTP works, which is what makes this option cloud-agnostic
  (no S3/GCS/Azure dependency at all). Optionally gated by `API_TOKEN` (see
  below).
- **`s3`** - `config_fetcher.py` polls an S3 bucket. `gatewayctl push-s3`
  uploads `rendered/` there instead. Useful once you want config shared
  across multiple gateway instances via a durable, versioned store.

## Quickstart (HTTP source)

```bash
make venv && source .venv/bin/activate
make all                       # generate SSL certs + build image
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
