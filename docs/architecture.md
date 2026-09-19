# Architecture

![architecture diagram](architecture.png)

## Dynamic Configuration

Clusters and routes are not static in `config/envoy.yaml`. The bootstrap
config only points `dynamic_resources.cds_config`/`lds_config` at
`/etc/envoy/dynamic/{cds,lds}.yaml` inside the container, with
`watched_directory` set so Envoy watches that directory via inotify.

`config/dynamic` is not bind-mounted from the host. On Docker Desktop for
Mac, host-side writes into a bind-mounted directory sync file content into
the container but do not reliably propagate the underlying inotify event,
so Envoy never notices the change even though the file is correct.

To work around this, the `fetcher/` binary (a statically linked Go binary)
runs inside the container, managed by `supervisor.ini`, and polls a remote
source for `cds.yaml`/`lds.yaml`. It writes them into `/etc/envoy/dynamic`
itself, a write native to the container's own filesystem, which does
trigger Envoy's inotify watch. `gatewayctl` on the host only ever writes to
`config/values.yaml` and the local `rendered/` directory; it never touches
the container's filesystem directly.

Both `gatewayctl` and the fetcher write atomically (temp file, then
rename, rather than delete-then-write), so a reader never observes a
missing or partial file mid-swap.

## Config Distribution

Two distribution mechanisms are supported, selected by the
`CONFIG_SOURCE_KIND` environment variable on the container. Both require
an explicit push after each change; `gatewayctl` never touches the
container or the fetcher's source directly, only the distribution
endpoint.

**`http`**

The fetcher polls `config_server`, a small Go HTTP service with its own
`storage/` directory. It has its own `Dockerfile`/`Makefile` and is
deployable as its own service. `gatewayctl push-http` uploads
`rendered/cds.yaml`/`lds.yaml` to it over HTTP POST. The server does not
need to be colocated with `gatewayctl`; anywhere reachable over HTTP
works, which is what makes this option cloud-agnostic (no dependency on
AWS or any other specific provider). Access is optionally gated by
`API_TOKEN` (see the main README).

**`s3`**

The fetcher polls an S3 bucket, so this mode is AWS-specific (or requires
an S3-compatible API). `gatewayctl push-s3` uploads `rendered/` there
instead. This is useful once configuration needs to be shared across
multiple gateway instances via a durable, versioned store.
