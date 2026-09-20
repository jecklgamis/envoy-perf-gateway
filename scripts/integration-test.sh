#!/usr/bin/env bash
# Integration test for the envoy-perf-gateway image: renders a values.yaml
# covering every add-backend feature (TLS, HTTP/2, host-rewrite, domain and
# path-prefix routing, gzip compression, a no-route backend), bakes it into
# a real image, boots it, and checks two things a Go unit test can't:
#
#   1. Envoy actually ACCEPTED the rendered config. Compiling and marshaling
#      cleanly is not the same as Envoy loading it - a wrong proto message
#      shape (see docs/architecture.md and CLAUDE.md's compressor writeup)
#      produces valid-looking YAML that Envoy still rejects at listener/
#      cluster-load time, silently, with no error anywhere except its own
#      admin API. This is exactly the bug class this script exists to catch
#      automatically instead of relying on someone noticing during manual
#      testing.
#   2. The features that affect response behavior actually behave as
#      configured (gzip only when asked for and only when the client
#      supports it, routes isolated from each other).
#
# Run this after any change to gatewayctl/internal/{render,envoyconfig},
# config/envoy.yaml, or the Dockerfile - anything that could change what
# gets fed to Envoy.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

IMAGE_TAG="envoy-perf-gateway:integration-test"
CONTAINER_NAME="envoy-perf-gateway-integration-test"
WORKDIR="$(mktemp -d)"
HTTP_PORT=18280
ADMIN_PORT=19291

# CRITICAL: without this, gatewayctl falls back to
# ~/.config/gatewayctl/config.yaml - whoever's real settings file that
# happens to be, on whatever machine runs this script. If that file has a
# mode configured (http/s3), every add-backend/remove-backend call below
# auto-pushes to a REAL, possibly production, config server - which is
# exactly what happened the first time this script was written, live on
# this project's own deployment. GATEWAYCTL_CONFIG must point at a path
# that is guaranteed not to exist (fresh mktemp dir, every run) so
# resolveMode() always resolves to "" and autoPushIfConfigured() is
# always a no-op here, no matter whose machine or CI runner this executes
# on. Do not remove this without a very good reason.
export GATEWAYCTL_CONFIG="$WORKDIR/gatewayctl-settings.yaml"

cleanup() {
  local exit_code=$?
  docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
  docker rmi "$IMAGE_TAG" >/dev/null 2>&1 || true
  rm -rf "$WORKDIR" "$PWD/rendered"
  exit "$exit_code"
}
trap cleanup EXIT

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1" >&2; exit 1; }

echo "--- Building gatewayctl ---"
(cd gatewayctl && go build -o gatewayctl .)

echo "--- Rendering a values.yaml covering every add-backend feature ---"
GATEWAY_BIN="$PWD/gatewayctl/gatewayctl"
VALUES="$WORKDIR/values.yaml"

# --force: no mode is configured (no --config pointed at a real settings
# file), so add-backend's missing-values.yaml guard would otherwise block
# every one of these on the very first call.
"$GATEWAY_BIN" add-backend --name httpbin --host httpbin.org --port 443 --tls \
  --route-prefix /httpbin/ --compression gzip \
  --values "$VALUES" --rendered-dir "$PWD/rendered" --force
"$GATEWAY_BIN" add-backend --name grpc-style --host httpbin.org --port 443 --tls --http2 \
  --domain grpc-style.integration-test.local \
  --values "$VALUES" --rendered-dir "$PWD/rendered"
"$GATEWAY_BIN" add-backend --name vhost --host httpbin.org --port 443 --tls \
  --domain vhost.integration-test.local --route-prefix /api/ --host-header httpbin.org \
  --values "$VALUES" --rendered-dir "$PWD/rendered"
"$GATEWAY_BIN" add-backend --name no-route --host internal.example --port 9000 \
  --values "$VALUES" --rendered-dir "$PWD/rendered"

echo "--- Building the image with this config baked in ---"
docker build -t "$IMAGE_TAG" . >/dev/null

echo "--- Starting the container ---"
docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
docker run -d --name "$CONTAINER_NAME" -p "$HTTP_PORT:8080" -p "$ADMIN_PORT:9901" "$IMAGE_TAG" >/dev/null

echo "--- Waiting for Envoy admin to come up ---"
for i in $(seq 1 30); do
  if curl -s -o /dev/null "http://localhost:$ADMIN_PORT/ready"; then
    break
  fi
  if [ "$i" = 30 ]; then
    fail "Envoy admin never came up"
  fi
  sleep 1
done

echo "--- Checking Envoy accepted the rendered config (the actual point of this test) ---"
STATS="$(curl -s "http://localhost:$ADMIN_PORT/stats")"
for metric in cluster_manager.cds.update_rejected cluster_manager.cds.update_failure \
              listener_manager.lds.update_rejected listener_manager.lds.update_failure; do
  value="$(echo "$STATS" | grep "^${metric}:" | awk '{print $2}')"
  if [ "$value" != "0" ]; then
    echo "$STATS" | grep "^${metric}:"
    curl -s "http://localhost:$ADMIN_PORT/config_dump?resource=dynamic_listeners" || true
    curl -s "http://localhost:$ADMIN_PORT/config_dump?resource=dynamic_active_clusters" || true
    fail "$metric = $value, want 0 - Envoy rejected part of the rendered config (see error_state dumps above)"
  fi
done
pass "cds/lds fully accepted (update_rejected and update_failure are 0)"

echo "--- Warming up upstream connections ---"
# These routes proxy to a real external site (httpbin.org) so Envoy's
# clusters get verified doing something real, not just accepting config -
# but that means each cluster's *first* request pays real DNS+TLS
# connection-establishment latency (observed: several seconds) before
# Envoy has a warm connection, and can occasionally come back 502 if that
# setup hasn't finished. Deliberately warm each route (retry until 200,
# discarding the result) before asserting anything, so a cold connection
# on request #1 isn't mistaken for a real routing/config bug. A route
# that's still failing after this many tries is a real problem.
warm_up() {
  local url="$1" host_header="${2:-}" attempt code
  for attempt in 1 2 3 4 5 6 7 8; do
    if [ -n "$host_header" ]; then
      code="$(curl -s -o /dev/null -w '%{http_code}' -H "Host: $host_header" "$url")"
    else
      code="$(curl -s -o /dev/null -w '%{http_code}' "$url")"
    fi
    [ "$code" = "200" ] && return 0
    sleep "$attempt"
  done
  fail "warming up $url (Host: ${host_header:-<none>}): still $code after $attempt attempts"
}
warm_up "http://localhost:$HTTP_PORT/httpbin/get"
warm_up "http://localhost:$HTTP_PORT/api/get" "vhost.integration-test.local"
pass "upstream connections warmed up"

echo "--- Functional checks ---"

code="$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:$HTTP_PORT/httpbin/get")"
[ "$code" = "200" ] || fail "path-prefix route: got $code, want 200"
pass "path-prefix route (httpbin) serves 200"

encoding="$(curl -s -D - -o /dev/null -H "Accept-Encoding: gzip" "http://localhost:$HTTP_PORT/httpbin/get" | grep -i '^content-encoding:' || true)"
echo "$encoding" | grep -qi gzip || fail "expected gzip content-encoding on the compressed route, got: ${encoding:-<none>}"
pass "gzip compression applies when the client supports it"

encoding="$(curl -s -D - -o /dev/null "http://localhost:$HTTP_PORT/httpbin/get" | grep -i '^content-encoding:' || true)"
[ -z "$encoding" ] || fail "expected no compression without Accept-Encoding, got: $encoding"
pass "gzip compression is skipped when the client doesn't support it"

code="$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:$HTTP_PORT/")"
[ "$code" = "200" ] || fail "default_app fallback route: got $code, want 200"
pass "default_app fallback route serves 200"

encoding="$(curl -s -D - -o /dev/null -H "Accept-Encoding: gzip" "http://localhost:$HTTP_PORT/" | grep -i '^content-encoding:' || true)"
[ -z "$encoding" ] || fail "compression must not leak to a route that didn't opt in (default_app), got: $encoding"
pass "gzip compression does not leak to a route that didn't opt in"

code="$(curl -s -o /dev/null -w '%{http_code}' -H "Host: vhost.integration-test.local" "http://localhost:$HTTP_PORT/api/get")"
[ "$code" = "200" ] || fail "domain+prefix virtual host route: got $code, want 200"
pass "domain+prefix virtual host route serves 200"

echo
echo "All integration checks passed."
