#!/bin/sh
export PULSE_SERVER=unix:/run/user/`id -u`/pulse/native

# WAYLAND_DISPLAY is normally a bare socket name (e.g. "wayland-0") that
# clients resolve relative to $XDG_RUNTIME_DIR. `snap run` always
# privatizes XDG_RUNTIME_DIR to a per-snap subdirectory
# (/run/user/<uid>/snap.<name>/) for this app, but the compositor
# (gnome-shell, started directly by systemd rather than via `snap run`)
# creates its socket under the real, unprivatized
# /run/user/<uid>/wayland-N. Rewrite a relative WAYLAND_DISPLAY to an
# absolute path pointing at the real socket so clients in this snap can
# still find it; libwayland treats an absolute WAYLAND_DISPLAY as a
# literal socket path instead of resolving it against XDG_RUNTIME_DIR.
case "$WAYLAND_DISPLAY" in
	/*|"") ;;
	*) export WAYLAND_DISPLAY="/run/user/`id -u`/$WAYLAND_DISPLAY" ;;
esac

exec "$@"
