#!/bin/sh

# If xdg-user-dirs-update exists in $PATH, run it
if command -v xdg-user-dirs-update >/dev/null; then
  xdg-user-dirs-update
fi

# Ensure socket directories exist and have the right permissions
mkdir -p /tmp/.X11-unix /tmp/.ICE-unix
chmod 01777 /tmp/.X11-unix /tmp/.ICE-unix

# Create the runtime directory
echo creating the runtime directory $XDG_RUNTIME_DIR
mkdir -p --mode=700 $XDG_RUNTIME_DIR

USERPATH=/run/user/`id -u`

export PULSE_SERVER=unix:$USERPATH/pulse/native
export WAYLAND_DISPLAY=$USERPATH/wayland-0
export GNOME_SHELL_SESSION_MODE=ubuntu
export PIPEWIRE_RUNTIME_DIR=$USERPATH

if ! grep "^snap$" $HOME/.hidden 2>&1 > /dev/null; then
  echo "snap" >> $HOME/.hidden
fi

#exec $SNAP/usr/bin/gnome-shell --display-server --wayland
$SNAP/usr/bin/gnome-session --builtin --session=ubuntu
# These must be deleted to ensure that another user can
# launch a session from GDM. Not doing it (or doing it from
# outside the snap) will prevent login with a different user
# than the first one that logged in, until the system is reboot.
rm -rf /tmp/.X11-unix
rm -rf /tmp/.ICE-unix
