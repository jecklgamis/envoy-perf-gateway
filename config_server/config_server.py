#!/usr/bin/env python3
"""Config distribution server: accepts rendered config files over HTTP
(`envoyctl push-http`) and serves the latest version back out
(`config_fetcher.py` polling inside the Envoy container).

This decouples envoyctl from the server's filesystem - the server keeps its
own storage and can run anywhere reachable over HTTP, not just colocated on
the same disk as envoyctl."""
import hmac
import os
import tempfile

from flask import Flask, abort, request, send_from_directory
from werkzeug.utils import secure_filename

STORAGE_DIR = os.environ.get(
    "CONFIG_SERVER_STORAGE_DIR",
    os.path.join(os.path.dirname(os.path.abspath(__file__)), "storage"),
)
os.makedirs(STORAGE_DIR, exist_ok=True)

# If unset, /config/* is unauthenticated - fine for local dev, not for a
# server reachable beyond localhost. /healthz is never gated, so liveness
# probes don't need the token.
API_TOKEN = os.environ.get("API_TOKEN")

app = Flask(__name__)


@app.before_request
def check_auth():
    if not API_TOKEN or not request.path.startswith("/config/"):
        return None
    header = request.headers.get("Authorization", "")
    prefix = "Bearer "
    token = header[len(prefix):] if header.startswith(prefix) else ""
    if not hmac.compare_digest(token, API_TOKEN):
        abort(401)
    return None


def atomic_write(path, data):
    directory = os.path.dirname(os.path.abspath(path))
    fd, tmp_path = tempfile.mkstemp(dir=directory, prefix=".tmp-")
    try:
        with os.fdopen(fd, "wb") as f:
            f.write(data)
        os.replace(tmp_path, path)
    except Exception:
        if os.path.exists(tmp_path):
            os.unlink(tmp_path)
        raise


@app.route("/config/<path:filename>", methods=["GET"])
def get_config(filename):
    filename = secure_filename(filename)
    if not os.path.isfile(os.path.join(STORAGE_DIR, filename)):
        abort(404)
    return send_from_directory(STORAGE_DIR, filename)


@app.route("/config/<path:filename>", methods=["POST"])
def upload_config(filename):
    filename = secure_filename(filename)
    if "file" not in request.files:
        abort(400, "missing 'file' field")
    data = request.files["file"].read()
    atomic_write(os.path.join(STORAGE_DIR, filename), data)
    return {"status": "ok", "filename": filename, "bytes": len(data)}


@app.route("/healthz")
def healthz():
    return {"status": "ok"}


if __name__ == "__main__":
    port = int(os.environ.get("PORT", "8090"))
    app.run(host="0.0.0.0", port=port)
