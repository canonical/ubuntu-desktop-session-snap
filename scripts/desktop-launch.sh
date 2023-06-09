#!/bin/bash

# Set $REALHOME to the users real home directory
REALHOME=$(getent passwd $UID | cut -d ':' -f 6)
export HOME=$REALHOME

# If xdg-user-dirs-update exists in $PATH, run it
if command -v xdg-user-dirs-update >/dev/null; then
  xdg-user-dirs-update
fi

exec "$@"
