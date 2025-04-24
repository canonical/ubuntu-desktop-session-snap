#!/bin/sh
USERPATH=/run/user/`id -u`
export PULSE_SERVER=unix:$USERPATH/pulse/native
export WAYLAND_DISPLAY=$USERPATH/wayland-0
export PIPEWIRE_RUNTIME_DIR=$USERPATH
exec "$@"
