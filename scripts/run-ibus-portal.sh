#!/bin/sh

# Systemd D-Bus activation does not inherit IBUS_ADDRESS from ibus-daemon.
# IBus writes it to a config file keyed by the display's bare socket name.
if [ -z "${IBUS_ADDRESS:-}" ]; then
	display=${WAYLAND_DISPLAY:-${DISPLAY:-}}
	display=${display##*/}
	if [ -n "$display" ] && [ -r /etc/machine-id ]; then
		IFS= read -r machine_id < /etc/machine-id
		bus_file="${XDG_CONFIG_HOME:-$HOME/.config}/ibus/bus/${machine_id}-unix-${display}"
		if [ -r "$bus_file" ]; then
			IBUS_ADDRESS=$(sed -n 's/^IBUS_ADDRESS=//p' "$bus_file")
		fi
	fi
fi

if [ -z "${IBUS_ADDRESS:-}" ]; then
	echo "Unable to locate the IBus address for display ${display:-<unset>}" >&2
	exit 1
fi

export IBUS_ADDRESS
exec "$SNAP/run.sh" /usr/libexec/ibus-portal
