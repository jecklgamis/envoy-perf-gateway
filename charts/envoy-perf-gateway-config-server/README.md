# envoy-perf-gateway-config-server

Standalone chart for `config_server` - the small HTTP service that stores
`cds.yaml`/`lds.yaml`/`runtime.yaml` pushed by `gatewayctl` and serves them
to one or more gateways' in-container fetchers. Deployed separately from
the gateway itself, same as running `config_server`'s own
`Dockerfile`/`Makefile` outside Kubernetes.

```bash
helm install my-config-server . --set apiToken=default
```

Then point a gateway at it - either the `envoy-perf-gateway` chart's
`gateway.configSourceUrl`, or `gatewayctl config set http.server-url`
directly - using its Service address,
`http://<release-name>.<namespace>.svc.cluster.local:<service.port>`.

## Values

| Key | Description |
| --- | --- |
| `apiToken` | Required; the server refuses to start without one. Prefer `apiTokenExistingSecret` beyond local/dev use. |
| `persistence.enabled` | Persists pushed config across restarts (default `true`, `ReadWriteOnce`). |
| `replicaCount` | Keep at `1` unless `persistence` is backed by a volume multiple pods can safely share - the Deployment strategy is `Recreate` specifically to avoid two writers to the same `ReadWriteOnce` volume. |
