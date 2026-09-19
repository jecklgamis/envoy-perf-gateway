# envoy-perf-gateway

Deploys the gateway and (optionally) the bundled config server.

```bash
helm install my-gw . \
  --set gateway.configApiToken=default \
  --set configServer.apiToken=default
```

## Distribution mode

`configSourceKind` (`http` or `s3`, default `http`) selects how the
gateway's backends and fault injection are distributed - see the main
repo's [architecture doc](../../docs/architecture.md). `http` also
controls whether `configServer.*` resources are deployed; set it to `s3`
and configure `gateway.s3.bucket` to skip the config server entirely.

## Values

See `values.yaml` for the full list. Notable ones:

| Key | Description |
| --- | --- |
| `configSourceKind` | `http` or `s3` |
| `gateway.replicaCount` | Gateway pod count. Fault injection and backends both converge across all replicas via the config pipeline (see main README's Fault Injection section). |
| `gateway.configApiToken` / `configServer.apiToken` | Required in `http` mode. Prefer the matching `*ExistingSecret` value beyond local/dev use. |
| `gateway.s3.bucket` | Required in `s3` mode. |
| `configServer.persistence.enabled` | Persists pushed config across config server restarts (default `true`). |

## Notes

- The gateway's admin port (`:9901`) is exposed on its Service for
  convenience, but a direct `POST /runtime_modify` against it only
  affects one pod and doesn't survive a restart - use `gatewayctl fault`
  for anything that should apply across `gateway.replicaCount` replicas.
- `configServer.persistence` uses `ReadWriteOnce`; the Deployment strategy
  is `Recreate` to avoid two writers to the same volume.
