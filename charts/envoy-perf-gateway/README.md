# envoy-perf-gateway

Deploys the gateway only. `config_server` is a separate chart,
[envoy-perf-gateway-config-server](../envoy-perf-gateway-config-server) -
install it independently (or point at an existing one, or use `s3` mode
instead) and set `configSourceUrl` to its address.

```bash
# 1. install the config server chart first (same or a different release)
helm install my-config-server ../envoy-perf-gateway-config-server --set apiToken=default

# 2. install this chart pointed at it
helm install my-gw . \
  --set configApiToken=default \
  --set configSourceUrl=http://my-config-server:8090
```

Or with `s3` mode, no config server needed:

```bash
helm install my-gw . \
  --set configSourceKind=s3 \
  --set s3.bucket=my-bucket
```

## Distribution mode

`configSourceKind` (`http` or `s3`, default `http`) selects how
backends and fault injection are distributed - see the main repo's
[architecture doc](../../docs/architecture.md).

## Values

See `values.yaml` for the full list. Notable ones:

| Key | Description |
| --- | --- |
| `configSourceKind` | `http` or `s3` |
| `replicaCount` | Gateway pod count. Fault injection and backends both converge across all replicas via the config pipeline (see main README's Fault Injection section). |
| `configSourceUrl` | Required in `http` mode - the config server's address. |
| `configApiToken` | Required in `http` mode; must match the config server's token. Prefer `configApiTokenExistingSecret` beyond local/dev use. |
| `s3.bucket` | Required in `s3` mode. |

## Notes

- The gateway's admin port (`:9901`) is exposed on its Service for
  convenience, but a direct `POST /runtime_modify` against it only
  affects one pod and doesn't survive a restart - use `gatewayctl fault`
  for anything that should apply across `replicaCount` replicas.
- The pod's `readinessProbe` targets the fetcher's own `:8081/ready`, not
  Envoy's admin API - it only reports ready after the fetcher's first
  successful sync of the real config source, so a pod isn't marked Ready
  and handed traffic while still serving the image's baked-in seed config
  (which is usually empty - `rendered/` is gitignored). Not exposed on the
  Service; it's a pod-internal probe target only.
