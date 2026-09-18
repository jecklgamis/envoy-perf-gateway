#!/usr/bin/env python3
"""Polls a remote source for cds.yaml/lds.yaml and atomically writes any
changes into the directory Envoy watches via inotify.

Runs INSIDE the Envoy container (see supervisor.ini) so the write is native
to the container's filesystem - this is what makes the inotify-based
hot-reload actually fire, unlike a host-side bind-mount write on Docker
Desktop for Mac, which syncs file content but does not propagate the
underlying filesystem event into the container.

Source kind is selected with CONFIG_SOURCE_KIND=http|s3.
"""
import hashlib
import logging
import os
import sys
import time

from atomic_write import atomic_write_bytes

CONFIG_FILES = os.environ.get("CONFIG_FILES", "cds.yaml,lds.yaml").split(",")
TARGET_DIR = os.environ.get("CONFIG_TARGET_DIR", "/etc/envoy/dynamic")
POLL_INTERVAL_SECONDS = float(os.environ.get("CONFIG_POLL_INTERVAL_SECONDS", "15"))
SOURCE_KIND = os.environ.get("CONFIG_SOURCE_KIND", "http")


def init_logging():
    logging.basicConfig(
        level=logging.INFO,
        format="[%(asctime)s] {config_fetcher.py} %(levelname)s - %(message)s",
        handlers=[logging.StreamHandler(sys.stdout)],
    )


def sha256(content):
    return hashlib.sha256(content).hexdigest()


class HttpSource:
    """Fetches rendered config files by HTTP GET from a base URL, e.g.
    a small Flask server that serves the `rendered/` directory produced by
    `gatewayctl regenerate()`."""

    def __init__(self):
        import requests
        self.requests = requests
        self.base_url = os.environ["CONFIG_SOURCE_URL"].rstrip("/")
        api_token = os.environ.get("CONFIG_API_TOKEN")
        self.headers = {"Authorization": f"Bearer {api_token}"} if api_token else {}

    def fetch(self, filename):
        url = f"{self.base_url}/config/{filename}"
        r = self.requests.get(url, headers=self.headers, timeout=5)
        r.raise_for_status()
        return r.content


class S3Source:
    """Fetches rendered config files from S3. Standard boto3 credential
    resolution applies (env vars, shared config, instance/task role)."""

    def __init__(self):
        import boto3
        self.bucket = os.environ["CONFIG_S3_BUCKET"]
        self.prefix = os.environ.get("CONFIG_S3_PREFIX", "").lstrip("/")
        self.client = boto3.client("s3", region_name=os.environ.get("AWS_REGION"))

    def fetch(self, filename):
        key = f"{self.prefix}{filename}" if self.prefix else filename
        obj = self.client.get_object(Bucket=self.bucket, Key=key)
        return obj["Body"].read()


def build_source(kind):
    if kind == "http":
        return HttpSource()
    if kind == "s3":
        return S3Source()
    raise ValueError(f"Unsupported CONFIG_SOURCE_KIND: {kind}")


def fetch_event_loop(source, target_dir, files, poll_interval):
    logging.info(f"Polling {SOURCE_KIND} every {poll_interval}s for {files} -> {target_dir}")
    last_hash = {}
    while True:
        for filename in files:
            try:
                content = source.fetch(filename)
            except Exception as e:
                logging.warning(f"Fetch failed for {filename}: {e}")
                continue
            digest = sha256(content)
            if last_hash.get(filename) == digest:
                logging.info(f"Fetched {filename} successfully ({len(content)} bytes, unchanged)")
                continue
            target_path = os.path.join(target_dir, filename)
            try:
                atomic_write_bytes(target_path, content)
                last_hash[filename] = digest
                logging.info(f"Fetched {filename} successfully, updated {target_path} ({len(content)} bytes)")
            except Exception as e:
                logging.warning(f"Failed to write {target_path}: {e}")
        time.sleep(poll_interval)


def main():
    init_logging()
    if not os.path.isdir(TARGET_DIR):
        logging.error(f"{TARGET_DIR} does not exist")
        sys.exit(1)
    source = build_source(SOURCE_KIND)
    fetch_event_loop(source, TARGET_DIR, CONFIG_FILES, POLL_INTERVAL_SECONDS)


if __name__ == "__main__":
    main()
