#!/bin/sh

# Ensure socket directories exist and have the right permissions
mkdir -p /tmp/.X11-unix /tmp/.ICE-unix
chmod 01777 /tmp/.X11-unix /tmp/.ICE-unix

# Create the runtime directory
mkdir -p --mode=700 $XDG_RUNTIME_DIR

export GNOME_SHELL_SESSION_MODE=ubuntu
exec /usr/bin/gnome-session --session=ubuntu
