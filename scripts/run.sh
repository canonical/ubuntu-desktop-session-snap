#!/bin/sh
export PULSE_SERVER=unix:/run/user/`id -u`/pulse/native
export SPA_PLUGIN_DIR=/usr/lib/x86_64-linux-gnu/spa-0.2
exec "$@"
