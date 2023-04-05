#!/bin/sh
export PULSE_SERVER=unix:/run/user/`id -u`/snap.ubuntu-desktop-session/pulse/native
export PULSE_RUNTIME_DIR=/run/user/`id -u`/snap.ubuntu-desktop-session
export PIPEWIRE_RUNTIME_DIR=/run/user/`id -u`/snap.ubuntu-desktop-session
export SPA_PLUGIN_DIR=/usr/lib/x86_64-linux-gnu/spa-0.2
exec "$@"
