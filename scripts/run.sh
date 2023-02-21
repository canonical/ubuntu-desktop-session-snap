#!/bin/sh
export PULSE_SERVER=unix:/run/user/`id -u`/pulse/native
export PIPEWIRE_RUNTIME_DIR=unix:/run/user/`id -u`
exec "$@"
