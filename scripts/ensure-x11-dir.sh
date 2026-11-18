#!/bin/sh
# T53: keep /tmp/.X11-unix acceptable to EVERY session uid of this snap.
#
# WHY: mutter (the gnome-shell process) validates the X11 socket
# directory when it spawns Xwayland (mutter src/wayland/meta-xwayland.c
# ensure_x11_unix_perms): the directory must be owned by the same uid
# as /tmp (root inside this snap's private tmp) or by the session's own
# uid, be writable, and carry the sticky bit -- otherwise the shell
# dies with "Failed to start X Wayland: Wrong ownership for directory
# /tmp/.X11-unix, owned by <uid> but should be same as /tmp ...".
#
# The directory lives in this snap's SHARED private tmp: snap-confine
# bind-mounts the host path /tmp/snap-private-tmp/snap.<snap>/tmp
# (root:root 01777) over /tmp for every process of this snap
# (cmd/snap-confine/mount-support.c setup_private_tmp), so ALL sessions
# share it regardless of uid -- the wizard's dynamic-uid mutter (e.g.
# 60578) creates it self-owned and the user session's mutter (1000)
# then refuses it and SIGABRTs. The host /tmp is shadowed by the bind,
# so host-side tmpfiles rules never see this directory.
#
# WHAT THIS DOES (best-effort, loud, never fatal):
# - missing            -> create it mode 1777 (owned by the caller).
# - owned by /tmp's uid (root here) or by the caller
#                      -> keep it; fix the mode to 1777 when the
#                         caller owns it (mutter needs sticky+writable).
# - foreign-owned      -> non-root callers attempt rm -rf + recreate;
#                         that is EXPECTED to fail with EPERM: the
#                         private tmp root is sticky root:root 01777
#                         (snap-confine enforces it) and the kernel
#                         check_sticky only lets the entry's owner,
#                         the directory's owner, or root remove it --
#                         in that case log the loud error + the root
#                         workaround and continue (mutter will then
#                         fail with its own "Wrong ownership" error,
#                         the pre-T53 behavior). Root callers (the
#                         ensure-x11-dir oneshot service) chown it to
#                         root instead: every uid accepts a root-owned
#                         dir, and chown keeps a live session's
#                         sockets intact.
#
# Called from run.sh's gnome-shell branch (the compositor is the
# process that runs mutter's check, so this runs once per session
# start, BEFORE mutter launches -- wizard, greeter and user shells all
# dispatch through that app) and from run-session.sh (the ubuntu
# session entry, before gnome-session spawns anything).

dir=/tmp/.X11-unix
uid=$(id -u)
tmp_owner=$(stat -c %u /tmp 2>/dev/null) || tmp_owner=0

log() {
	printf 'ensure-x11-dir: %s\n' "$*" >&2
	# Best-effort file log for the launcher flow (the
	# wizard-a11y-chord.log precedent); the root oneshot service has
	# no XDG_RUNTIME_DIR, it logs to its unit journal only.
	if [ -n "$XDG_RUNTIME_DIR" ] && [ -d "$XDG_RUNTIME_DIR" ]; then
		printf '%s %s\n' "$(date +%H:%M:%S.%N)" "$*" \
			>>"$XDG_RUNTIME_DIR/x11-dir-self-heal.log" 2>/dev/null || true
	fi
}

if [ -d "$dir" ]; then
	owner=$(stat -c %u "$dir" 2>/dev/null)
	mode=$(stat -c %a "$dir" 2>/dev/null)
	if [ "$owner" = "$tmp_owner" ] || [ "$owner" = "$uid" ]; then
		# Acceptable to mutter (same uid as /tmp, or ours). mutter
		# additionally requires sticky + writable, so make sure the
		# mode is 1777 when we can change it.
		if [ "$mode" != "1777" ]; then
			if chmod 1777 "$dir" 2>/dev/null; then
				log "$dir was mode $mode (owner $owner): chmod 1777 ok"
			else
				log "WARNING $dir is mode $mode (owner $owner): chmod 1777 failed (EPERM) -- mutter rejects non-sticky dirs ('missing the sticky bit')"
			fi
		else
			log "$dir ok (owner $owner, mode 1777)"
		fi
	elif [ "$uid" = 0 ]; then
		# Root (the ensure-x11-dir oneshot service): chown is
		# non-destructive -- a live session's sockets keep working --
		# and makes the dir acceptable to every uid.
		if chown 0:0 "$dir" 2>/dev/null; then
			chmod 1777 "$dir" 2>/dev/null || true
			log "$dir was foreign-owned by $owner: chown root:root + chmod 1777 ok"
		elif rm -rf "$dir" 2>/dev/null && mkdir -m 1777 "$dir" 2>/dev/null; then
			log "$dir was foreign-owned by $owner: removed + recreated root-owned (chown refused)"
		else
			log "ERROR cannot normalize foreign-owned $dir (owner $owner) even as root"
		fi
	else
		# Non-root, foreign-owned: mutter would reject it. Try the
		# removal per the T53 spec; EPERM is the expected outcome for
		# a non-owner in the sticky root-owned private tmp.
		log "$dir foreign-owned by $owner (self $uid, /tmp owner $tmp_owner): mutter would reject it; attempting removal"
		if rm -rf "$dir" 2>/dev/null && [ ! -e "$dir" ]; then
			if mkdir -m 1777 "$dir" 2>/dev/null; then
				log "$dir removed + recreated (owner $uid, mode 1777)"
			else
				log "WARNING $dir removed but recreation failed"
			fi
		else
			log "ERROR cannot remove foreign-owned $dir (EPERM expected: sticky root-owned private tmp; only the entry owner or root may remove)"
			log "ERROR the session's mutter will abort with its 'Wrong ownership' error -- root workaround: chown root:root /proc/<session-shell-pid>/root/tmp/.X11-unix (the /proc chown recipe), or ensure the ubuntu-desktop-session.ensure-x11-dir oneshot service runs at boot"
		fi
	fi
else
	if mkdir -m 1777 "$dir" 2>/dev/null; then
		log "$dir missing: created (owner $uid, mode 1777)"
	else
		log "WARNING $dir missing and creation failed"
	fi
fi

exit 0
