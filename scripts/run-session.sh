#!/bin/sh

# If xdg-user-dirs-update exists in $PATH, run it
if command -v xdg-user-dirs-update >/dev/null; then
  xdg-user-dirs-update
fi

# Ensure socket directories exist and have the right permissions
mkdir -p /tmp/.X11-unix /tmp/.ICE-unix
chmod 01777 /tmp/.X11-unix /tmp/.ICE-unix

# Create the runtime directory
mkdir -p --mode=700 $XDG_RUNTIME_DIR

export PULSE_SERVER=unix:/run/user/`id -u`/pulse/native
export GNOME_SHELL_SESSION_MODE=ubuntu
export XDG_CURRENT_DESKTOP=ubuntu:GNOME

if ! grep "^snap$" $HOME/.hidden 2>&1 > /dev/null; then
  echo "snap" >> $HOME/.hidden
fi

exec /usr/bin/gnome-session --builtin --session=ubuntu
