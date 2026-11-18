#!/bin/sh
# T51: the wizard-session init gate.
#
# The gnome-session-service app command (snapcraft.yaml) was
#   run.sh $SNAP/usr/libexec/ubuntu-desktop-session/gnome-session-service
# and is now wrapped by THIS gate. run.sh exports the session env
# (PULSE_SERVER, fontconfig, the absolute WAYLAND_DISPLAY rewrite) and
# execs "$@" — so the gate sees:
#   $1 = $SNAP/usr/libexec/ubuntu-desktop-session/gnome-session-service
#   $2 = --session=gnome-initial-setup   (the systemd template arg
#        from gnome-session-manager@gnome-initial-setup.service; the
#        split form "--session gnome-initial-setup" is handled too)
#
# For the WIZARD session only, a detached init helper is started
# BEFORE the session manager execs:
#  - seed the gsd media-keys custom keybinding <Super><Alt>s -> the
#    in-snap toggle-screen-reader script into the wizard session's
#    dconf store (the stock shell chord is not bound in initial-setup
#    mode — the Shqip type-ahead leak),
#  - resolve the wizard session's mutter Xwayland display + auth file
#    and export DISPLAY/XAUTHORITY/NO_AT_BRIDGE into the systemd USER
#    MANAGER environment (+ the bus activation env) so every later
#    start of orca.service — gsd-a11y-settings' StartUnit — inherits
#    a display (orca dies at Gdk keymap init without one, T16).
# The normal user session (--session=ubuntu) and the greeter
# (--session=gnome-login) pass through with no seeding and no
# brokerage: the user session keeps the stock chord behavior and the
# greeter is Wayland-only (--no-x11, never an Xwayland).
_wizard=false
_prev=
for _a in "$@"; do
	if [ "$_prev" = "--session" ]; then
		[ "$_a" = "gnome-initial-setup" ] && _wizard=true
	elif [ "$_a" = "--session=gnome-initial-setup" ]; then
		_wizard=true
	fi
	_prev=$_a
done

if [ "$_wizard" = true ]; then
	"$SNAP/wizard-a11y-init.sh" \
		< /dev/null \
		>> "${XDG_RUNTIME_DIR:-/run/user/$(id -u)}/wizard-a11y-init.log" 2>&1 &
	# Detach: the helper outlives this exec (reparented to the user
	# manager) and must not hold the session manager's stdin/stdout.
	unset _wizard _prev _a
fi

exec "$@"
