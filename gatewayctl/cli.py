#!/usr/bin/env python3
import os
import sys

import click
import requests
import yaml

from gatewayctl.atomic_write import atomic_write
from gatewayctl.render import load_values, render

ROOT_DIR = os.getcwd()
DEFAULT_VALUES_PATH = os.path.join(ROOT_DIR, "values.yaml")
# Rendered files are the distribution source of truth - NOT bind-mounted
# into the container. They're picked up by config_fetcher.py running inside
# the container, either over HTTP (config_server/config_server.py serves
# this directory) or from S3 (`gatewayctl push-s3` uploads it there). The
# fetcher writes into /etc/envoy/dynamic *inside* the container, which is
# what actually triggers Envoy's inotify-based reload - a host-side
# bind-mount write does not, on Docker Desktop for Mac.
DEFAULT_RENDERED_DIR = os.path.join(ROOT_DIR, "rendered")


def save_values(values_path, values):
    with open(values_path, "w") as f:
        yaml.safe_dump(values, f, sort_keys=False)


def regenerate(values_path, rendered_dir):
    values = load_values(values_path)
    os.makedirs(rendered_dir, exist_ok=True)
    cds = render("cds.yaml.j2", values)
    lds = render("lds.yaml.j2", values)
    atomic_write(os.path.join(rendered_dir, "cds.yaml"), cds)
    atomic_write(os.path.join(rendered_dir, "lds.yaml"), lds)
    click.echo(f"Regenerated {rendered_dir}/cds.yaml and lds.yaml")


@click.group()
@click.option("--values", "values_path", default=DEFAULT_VALUES_PATH,
              envvar="GATEWAYCTL_VALUES", show_default=True,
              type=click.Path(dir_okay=False),
              help="Path to values.yaml. Defaults to values.yaml in the "
                   "current working directory.")
@click.option("--rendered-dir", default=DEFAULT_RENDERED_DIR,
              envvar="GATEWAYCTL_RENDERED_DIR", show_default=True,
              type=click.Path(file_okay=False),
              help="Directory to write rendered cds.yaml/lds.yaml into.")
@click.pass_context
def cli(ctx, values_path, rendered_dir):
    """envoy-perf-gateway control CLI"""
    ctx.obj = {"values_path": values_path, "rendered_dir": rendered_dir}


@cli.command("render")
@click.pass_obj
def render_cmd(obj):
    """Render cds.yaml and lds.yaml from values.yaml without changing any
    backend. The rendered directory is gitignored (generated), so this is
    the step CI runs before `docker build` on a fresh checkout."""
    regenerate(obj["values_path"], obj["rendered_dir"])


@cli.command("add-backend")
@click.option("--name", required=True, help="Cluster name, must be unique")
@click.option("--host", required=True, help="Upstream host/IP")
@click.option("--port", required=True, type=int)
@click.option("--tls/--no-tls", default=False, help="Terminate TLS to the upstream")
@click.option("--route-prefix", default=None,
              help="Path prefix routed to this backend (rewritten to /). "
                   "Omit to only add the cluster without a route.")
@click.option("--connect-timeout", default="5s")
@click.option("--timeout", default="15s")
@click.pass_obj
def add_backend(obj, name, host, port, tls, route_prefix, connect_timeout, timeout):
    """Add (or replace) a backend and hot-reload Envoy - no restart."""
    values = load_values(obj["values_path"])
    backends = values.setdefault("backends", [])
    backends[:] = [b for b in backends if b["name"] != name]
    backends.append({
        "name": name,
        "host": host,
        "port": port,
        "tls": tls,
        "route_prefix": route_prefix,
        "connect_timeout": connect_timeout,
        "timeout": timeout,
    })
    save_values(obj["values_path"], values)
    regenerate(obj["values_path"], obj["rendered_dir"])
    click.echo(f"Added backend '{name}' -> {host}:{port}"
               + (f" (routed from {route_prefix})" if route_prefix else ""))


@cli.command("remove-backend")
@click.option("--name", required=True)
@click.pass_obj
def remove_backend(obj, name):
    """Remove a backend and hot-reload Envoy."""
    values = load_values(obj["values_path"])
    backends = values.setdefault("backends", [])
    before = len(backends)
    backends[:] = [b for b in backends if b["name"] != name]
    if len(backends) == before:
        click.echo(f"No backend named '{name}' found", err=True)
        sys.exit(1)
    save_values(obj["values_path"], values)
    regenerate(obj["values_path"], obj["rendered_dir"])
    click.echo(f"Removed backend '{name}'")


@cli.command("list-backends")
@click.pass_obj
def list_backends(obj):
    values = load_values(obj["values_path"])
    backends = values.get("backends", [])
    if not backends:
        click.echo("No backends configured")
        return
    for b in backends:
        route = b.get("route_prefix") or "(no route, cluster only)"
        tls = "tls" if b.get("tls") else "plaintext"
        click.echo(f"{b['name']:<20} {b['host']}:{b['port']:<6} {tls:<10} {route}")


@cli.command("push-http")
@click.option("--server-url", default="http://localhost:8090", envvar="CONFIG_SERVER_URL",
              show_default=True,
              help="Base URL of a running config_server.")
@click.option("--api-token", default=None, envvar="CONFIG_SERVER_API_TOKEN",
              help="Sent as 'Authorization: Bearer <token>'. Required if the "
                   "server was started with API_TOKEN set.")
@click.pass_obj
def push_http(obj, server_url, api_token):
    """Upload rendered cds.yaml and lds.yaml to a running config_server
    over HTTP. Use this whenever the server isn't colocated with gatewayctl on
    the same filesystem - e.g. it's deployed separately from wherever you
    run the CLI. Regenerates first."""
    regenerate(obj["values_path"], obj["rendered_dir"])
    server_url = server_url.rstrip("/")
    headers = {"Authorization": f"Bearer {api_token}"} if api_token else {}
    for filename in ("cds.yaml", "lds.yaml"):
        local_path = os.path.join(obj["rendered_dir"], filename)
        with open(local_path, "rb") as f:
            r = requests.post(f"{server_url}/config/{filename}",
                               files={"file": (filename, f)}, headers=headers, timeout=10)
        r.raise_for_status()
        click.echo(f"Uploaded {local_path} -> {server_url}/config/{filename}")


@cli.command("push-s3")
@click.option("--bucket", required=True, envvar="CONFIG_S3_BUCKET")
@click.option("--prefix", default="", envvar="CONFIG_S3_PREFIX",
              help="Key prefix, e.g. 'envoy-perf-gateway/'")
@click.pass_obj
def push_s3(obj, bucket, prefix):
    """Upload rendered cds.yaml and lds.yaml to S3 for the in-container
    fetcher to pick up (CONFIG_SOURCE_KIND=s3). Regenerates first."""
    import boto3
    regenerate(obj["values_path"], obj["rendered_dir"])
    client = boto3.client("s3")
    prefix = prefix.lstrip("/")
    for filename in ("cds.yaml", "lds.yaml"):
        local_path = os.path.join(obj["rendered_dir"], filename)
        key = f"{prefix}{filename}" if prefix else filename
        client.upload_file(local_path, bucket, key)
        click.echo(f"Uploaded {local_path} -> s3://{bucket}/{key}")


@cli.group()
def fault():
    """Toggle fault injection at runtime via the Envoy admin API.

    No config reload involved - these hit /runtime_modify on the admin
    port directly, so changes take effect on the next request.
    """


@fault.command("abort")
@click.option("--percent", required=True, type=int, help="0-100")
@click.option("--status", default=503, type=int, help="HTTP status to return")
@click.option("--admin-url", default="http://localhost:9901", envvar="ENVOY_ADMIN_URL")
def fault_abort(percent, status, admin_url):
    url = (f"{admin_url.rstrip('/')}/runtime_modify"
           f"?fault.http.abort.abort_percent={percent}"
           f"&fault.http.abort.http_status={status}")
    r = requests.post(url, timeout=5)
    click.echo(f"{r.status_code} abort_percent={percent} status={status}")


@fault.command("delay")
@click.option("--percent", required=True, type=int, help="0-100")
@click.option("--duration-ms", required=True, type=int)
@click.option("--admin-url", default="http://localhost:9901", envvar="ENVOY_ADMIN_URL")
def fault_delay(percent, duration_ms, admin_url):
    url = (f"{admin_url.rstrip('/')}/runtime_modify"
           f"?fault.http.delay.delay_percent={percent}"
           f"&fault.http.delay.fixed_duration_ms={duration_ms}")
    r = requests.post(url, timeout=5)
    click.echo(f"{r.status_code} delay_percent={percent} duration_ms={duration_ms}")


@fault.command("reset")
@click.option("--admin-url", default="http://localhost:9901", envvar="ENVOY_ADMIN_URL")
def fault_reset(admin_url):
    url = (f"{admin_url.rstrip('/')}/runtime_modify"
           f"?fault.http.abort.abort_percent=0&fault.http.delay.delay_percent=0")
    r = requests.post(url, timeout=5)
    click.echo(f"{r.status_code} fault injection reset to 0%")


if __name__ == "__main__":
    cli()
