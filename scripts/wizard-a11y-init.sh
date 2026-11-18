#!/bin/sh
# T51: the wizard-session a11y init helper (detached by
# wizard-session-init.sh). Gated on the initial-setup session by the
# caller. THE DISPLAY/AUTH BROKERAGE:
#
# Poll (bounded 30s) for the wizard session's mutter Xwayland socket +
# auth file, validate the pair with a real X connect, and export
# DISPLAY/XAUTHORITY/NO_AT_BRIDGE into the systemd USER MANAGER
# environment (systemctl --user set-environment; SetEnvironment is in
# the systemd-user-control grant list) and into the D-Bus activation
# environment (best-effort). orca.service is started LATER — on the
# chord, when gsd-a11y-settings StartUnits it — and a systemd user
# unit's environment is the manager env at START time, so the
# chord-time start inherits the brokered display. This is the
# permanent form of the manual recipe's DISPLAY=:1024 +
# XAUTHORITY=.mutter-Xwaylandauth.* step (the [2026-11-18] spike
# entry); without it orca dies at Gdk keymap init (T16).
#
# DESIGN NOTE — the alternative considered: a PRIVATE orca-owned
# Xwayland spawned via `Xwayland -wayland <socket>` by the shim, so
# orca never needs the session's display at all. REJECTED: the
# session's own Xwayland shares the seat keyboard with the wizard
# compositor, so orca's key grabs land on the same input path the
# user is typing on (and the grabs demonstrably work in that shape —
# the manual recipe's live session had working navigation); a private
# X server's keyboard routing for a wayland-session orca was
# UNVERIFIED, and a second X server per wizard session is a new
# privilege surface for no proven gain. The brokerage reuses the
# display the wizard session already owns.

_log_tag() { date '+%T.%N'; }
_uid=$(id -u)
_runtime="/run/user/$_uid"

# The bus to act on: the launcher env's DBUS_SESSION_BUS_ADDRESS
# (snapd points session-snap apps at the real session bus).
_bus_wait=0
while [ $_bus_wait -lt 20 ]; do
	_addr="${DBUS_SESSION_BUS_ADDRESS:-unix:path=$_runtime/bus}"
	_bpath=${_addr#unix:path=}
	[ -n "$_bpath" ] && [ -S "$_bpath" ] && break
	_bus_wait=$((_bus_wait + 1))
	sleep 1
done
echo "$(date '+%T.%N') wizard-a11y-init: bus after ${_bus_wait}s addr=$_addr" >&2

# ---------- the display/auth brokerage ----------
_deadline=$(( $(date +%s) + 30 ))
_auth=""
while [ "$(date +%s)" -lt "$_deadline" ]; do
	_auth=$(ls -t "$_runtime"/.mutter-Xwaylandauth.* 2>/dev/null | head -n 1)
	[ -n "$_auth" ] && break
	sleep 1
done
if [ -z "$_auth" ]; then
	echo "$(date '+%T.%N') brokerage: NO mutter auth file within 30s — giving up" >&2
	exit 1
fi
echo "$(date '+%T.%N') brokerage: auth=$_auth" >&2

_display=""
for _sock in $(ls -t /tmp/.X11-unix/X[0-9]* 2>/dev/null); do
	_num=${_sock##*/X}
	case $_num in ''|*[!0-9]*) continue ;; esac
	if timeout 3 env DISPLAY=":$_num" XAUTHORITY="$_auth" \
		xset q >/dev/null 2>&1; then
		_display=":$_num"
		break
	fi
	echo "$(date '+%T.%N') brokerage: socket :$_num rejected the auth file" >&2
done
if [ -z "$_display" ]; then
	echo "$(date '+%T.%N') brokerage: no socket validated against the auth file" >&2
	exit 1
fi
echo "$(date '+%T.%N') brokerage: validated $_display with $_auth" >&2

# Export into the systemd USER MANAGER env (orca.service inherits the
# manager env at START time) and the bus activation env (best-effort:
# the activation env covers non-systemd spawns).
systemctl --user set-environment \
	DISPLAY="$_display" XAUTHORITY="$_auth" NO_AT_BRIDGE=0
_rc=$?
echo "$(date '+%T.%N') brokerage: systemctl set-environment rc=$_rc" >&2
if command -v dbus-update-activation-environment >/dev/null 2>&1; then
	dbus-update-activation-environment --systemd \
		DISPLAY XAUTHORITY NO_AT_BRIDGE >&2 || \
		echo "$(date '+%T.%N') brokerage: dbus-update-activation-environment failed (non-fatal)" >&2
else
	echo "$(date '+%T.%N') brokerage: dbus-update-activation-environment not present" >&2
fi
echo "$(date '+%T.%N') brokerage: done display=$_display" >&2
