#!/usr/bin/env python3
"""Default echo backend: returns request metadata as JSON. Used as
`default_app` in cds.yaml.j2/lds.yaml.j2 - the catch-all cluster any route
without a more specific backend falls through to."""
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
    print(f"[app.py:{sys.argv[1]}] {request_data}", flush=True)
    return jsonify({"ok": "true", "request": request_data})


if __name__ == "__main__":
    if len(sys.argv) <= 1:
        print("Requires port number")
        sys.exit(1)
    app.run(host="0.0.0.0", port=int(sys.argv[1]))
