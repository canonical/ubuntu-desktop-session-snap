#!/bin/sh
export PULSE_SERVER=unix:/run/user/`id -u`/pulse/native
exec "$@"
