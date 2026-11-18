#!/bin/sh
# T51: the wizard-session init gate.
#
# TWO hook points share this gate, because the wizard session reaches
# the session snap through two different paths (verified live on the
# nouser bake):
#  1. gnome-session-service (the app command is wrapped as
#     run.sh wizard-session-init.sh $SNAP/usr/libexec/.../gnome-session-service):
#     the ubuntu user session passes here
#     (gnome-session-manager@ubuntu.service.d -> gnome-content-launch
#     --session-dispatch -> the snap app) with the systemd template
#     arg --session=gnome-initial-setup ... for the wizard IF a base
#     drop-in ever redirects the wizard manager at the snap app (the
#     content default alias branch execs the CONTENT binary directly,
#     bypassing the snap — the live wizard session does NOT pass
#     here).
#  2. THE LIVE WIZARD PATH: the gsd apps — the base's
#     org.gnome.SettingsDaemon.{A11ySettings,MediaKeys}.service.d/
#     core-override.conf drop-ins redirect ExecStart at
#     /snap/bin/ubuntu-desktop-session.gsd-{a11y-settings,media-keys},
#     so those two ARE session-snap apps in the wizard session. Their
#     commands are wrapped as run.sh wizard-session-init.sh
#     /usr/libexec/gsd-... — the gate detects the wizard by ACCOUNT
#     NAME (id -un = gnome-initial-setup; the greeter user is
#     gdm-greeter, regular users are their own accounts), not by
#     argv, and replicates run.sh's /usr/* -> $SNAP/gnome/usr/*
#     content remap before exec.
#
# For the WIZARD session only, a detached init helper is started
# BEFORE the app execs:
#  - seed the gsd media-keys custom keybinding <Super><Alt>s -> the
#    in-snap toggle-screen-reader script into the wizard session's
#    dconf store (the stock shell chord is not bound in initial-setup
#    mode — the Shqip type-ahead leak),
#  - resolve the wizard session's mutter Xwayland display + auth file
#    and export DISPLAY/XAUTHORITY/NO_AT_BRIDGE into the systemd USER
#    MANAGER environment (+ the bus activation env) so every later
#    start of orca.service — gsd-a11y-settings' StartUnit — inherits
#    a display (orca dies at Gdk keymap init without one, T16).
# The helper is idempotent (both gsd apps run it; wizard-a11y-init.sh
# flock-guards itself) and the seeding writes identical values.
# The normal user session and the greeter pass through with no
# seeding and no brokerage: the user session keeps the stock chord
# behavior and the greeter is Wayland-only (--no-x11, never an
# Xwayland).
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
# Hook-point 2: no session arg reaches the gsd apps; the wizard
# session is identified by the running account (the dynamic sysusers
# uid is shared between gdm-greeter and gnome-initial-setup across
# boots — the NAME is the discriminator, the uid is not).
[ "$_wizard" = false ] && [ "$(id -un 2>/dev/null)" = "gnome-initial-setup" ] && _wizard=true

if [ "$_wizard" = true ]; then
	"$SNAP/wizard-a11y-init.sh" \
		< /dev/null \
		>> "${XDG_RUNTIME_DIR:-/run/user/$(id -u)}/wizard-a11y-init.log" 2>&1 &
	# Detach: the helper outlives this exec (reparented to the user
	# manager) and must not hold the session manager's stdin/stdout.
fi
unset _wizard _prev _a

# Replicate run.sh's content remap so the wrapped /usr/libexec/gsd-*
# paths resolve against the gnome-desktop-runtime content mount (the
# thin base ships no gsd binaries).
case "$1" in
	/usr/*)
		content_command="$SNAP/gnome$1"
		if [ -x "$content_command" ]; then
			shift
			set -- "$content_command" "$@"
		fi
		;;
esac

exec "$@"

