#!/bin/sh

# If xdg-user-dirs-update exists in $PATH, run it
if command -v xdg-user-dirs-update >/dev/null; then
  xdg-user-dirs-update
fi

# T53: the full /tmp/.X11-unix self-heal (see ensure-x11-dir.sh for the
# why: the dir lives in the snap's shared private tmp and the previous
# session may have left it foreign-owned, which makes the user session's
# mutter SIGABRT). This is the early pass before gnome-session spawns
# anything; the session's compositor re-runs the heal via run.sh's
# gnome-shell branch, and the ensure-x11-dir oneshot service is the
# boot-time root pass. The old bare `mkdir -p` + `chmod 01777` pair
# silently no-oped/EPERM'd on a foreign-owned dir (chmod by a non-owner
# is denied regardless of sticky) -- exactly the pre-T53 failure path.
"$SNAP/ensure-x11-dir.sh" || true
# .ICE-unix has no observed cross-uid failure (libICE does not run
# mutter's ownership check); keep the legacy creation.
mkdir -p /tmp/.ICE-unix
chmod 01777 /tmp/.ICE-unix

# Create the runtime directory
mkdir -p --mode=700 $XDG_RUNTIME_DIR

. "$SNAP/fontconfig-env.sh" || exit 1
. "$SNAP/gdk-pixbuf-env.sh" || exit 1

# T46: resolve the pulse socket that exists (see scripts/run.sh for
# the full rationale: the conventional path is only bound/hardlinked in
# real user sessions; greeter/wizard sessions have the snap-private
# socket only).
_pulse_uid=`id -u`
if [ -S "/run/user/$_pulse_uid/pulse/native" ]; then
	export PULSE_SERVER=unix:/run/user/$_pulse_uid/pulse/native
else
	export PULSE_SERVER=unix:/run/user/$_pulse_uid/snap.pipewire/pulse/native
fi
export GNOME_SHELL_SESSION_MODE=ubuntu
export XDG_CURRENT_DESKTOP=ubuntu:GNOME

if ! grep "^snap$" $HOME/.hidden 2>&1 > /dev/null; then
  echo "snap" >> $HOME/.hidden
fi

exec "$SNAP/usr/bin/gnome-session" --session=ubuntu
