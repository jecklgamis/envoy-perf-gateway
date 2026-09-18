#!/usr/bin/env python3
"""Default echo backend: returns request metadata as JSON. Used as
`default_app` in cds.yaml.j2/lds.yaml.j2 - the catch-all cluster any route
without a more specific backend falls through to.

Served by gunicorn in the container (see run-app.sh); the __main__ block
below is only for standalone local runs."""
import os
import sys

from flask import Flask, jsonify, request

app = Flask(__name__)


@app.route("/", defaults={"path": ""}, methods=["GET", "POST", "PUT", "DELETE", "PATCH"])
@app.route("/<path:path>", methods=["GET", "POST", "PUT", "DELETE", "PATCH"])
def echo(path):
    request_data = {
        "remote_ip": request.remote_addr,
        "method": request.method,
        "path": "/" + path,
        "headers": dict(request.headers),
        "query": request.query_string.decode() or None,
        "body": request.get_data(as_text=True),
    }
    print(f"[app.py] {request_data}", flush=True)
    return jsonify({"ok": "true", "request": request_data})


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else int(os.environ.get("PORT", "5050"))
    app.run(host="0.0.0.0", port=port)
