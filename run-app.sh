#!/usr/bin/env bash
exec gunicorn --chdir / --workers 1 --bind 0.0.0.0:5050 app:app
