# envoy-perf-gateway

Deploys the gateway only. `config_server` is a separate chart,
[envoy-perf-gateway-config-server](../envoy-perf-gateway-config-server) -
install it independently (or point at an existing one, or use `s3` mode
instead) and set `gateway.configSourceUrl` to its address.

```bash
# 1. install the config server chart first (same or a different release)
helm install my-config-server ../envoy-perf-gateway-config-server --set apiToken=default

# 2. install this chart pointed at it
helm install my-gw . \
  --set gateway.configApiToken=default \
  --set gateway.configSourceUrl=http://my-config-server:8090
```

Or with `s3` mode, no config server needed:

```bash
helm install my-gw . \
  --set gateway.configSourceKind=s3 \
  --set gateway.s3.bucket=my-bucket
```

## Distribution mode

`gateway.configSourceKind` (`http` or `s3`, default `http`) selects how
backends and fault injection are distributed - see the main repo's
[architecture doc](../../docs/architecture.md).

## Values

See `values.yaml` for the full list. Notable ones:

| Key | Description |
| --- | --- |
| `gateway.configSourceKind` | `http` or `s3` |
| `gateway.replicaCount` | Gateway pod count. Fault injection and backends both converge across all replicas via the config pipeline (see main README's Fault Injection section). |
| `gateway.configSourceUrl` | Required in `http` mode - the config server's address. |
| `gateway.configApiToken` | Required in `http` mode; must match the config server's token. Prefer `configApiTokenExistingSecret` beyond local/dev use. |
| `gateway.s3.bucket` | Required in `s3` mode. |

## Notes

- The gateway's admin port (`:9901`) is exposed on its Service for
  convenience, but a direct `POST /runtime_modify` against it only
  affects one pod and doesn't survive a restart - use `gatewayctl fault`
  for anything that should apply across `gateway.replicaCount` replicas.
