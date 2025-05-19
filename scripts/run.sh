#!/bin/sh
USERPATH=/run/user/`id -u`
export PULSE_SERVER=unix:$USERPATH/pulse/native
export PIPEWIRE_RUNTIME_DIR=$USERPATH
exec "$@"
