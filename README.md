# Ubuntu Desktop Session snap for Ubuntu Core Desktop

Provides a strictly confined desktop session for Ubuntu Core Desktop

## Session environment broker

`gnome-session-service` forwards Mutter's `Setenv` updates through the
`session-environment-broker` D-Bus API. Only that app is authorized to call
the API; the broker is the only session-snap app with the
`systemd-user-environment` plug. It accepts a fixed display/session/locale
allowlist, forces `XDG_RUNTIME_DIR` and `XAUTHORITY` to host-visible paths,
and updates both user systemd and D-Bus activation environments. It
authenticates the D-Bus sender's kernel-reported AppArmor label and UID,
so other apps in the session snap cannot use the API even though they
share limited intra-snap D-Bus access. Since systemd 259 does not expose
environment reads on its user D-Bus API, the broker clears its own
`SNAP_*` variables on each update; the unconfined session wrapper removes
stale names from both environments before startup.
The D-Bus activation API has no unset operation, so removed names are
written with empty values there. Because D-Bus activation updates also
seed the systemd manager on this image, the broker applies the explicit
systemd update last to unset removed names. The existing wrapper pre-seed
and Xauthority-copy behavior remain in place.

The GNOME Session bulk exports are intentionally no-ops for the shared
environment in the patched `gnome-session-service`: the unconfined
session wrapper seeds host values before startup, while compositor
changes use the broker. Empty Setenv values are forwarded as unsets.
The GTK portal backend is registered as a D-Bus-activated user service
so the portal frontend can start it when requested.
The separate confined bootstrap process still has legacy direct-write
attempts, which AppArmor denies; it cannot mutate either shared
environment. The session's child environment remains unchanged.

The `systemd-user-control` interface retains unit control and inspection
rules for the session. It no longer grants `UpdateActivationEnvironment`,
`SetEnvironment`, `UnsetEnvironment`, or `UnsetAndSetEnvironment`; those
shared-environment writes are isolated in `systemd-user-environment`.
The source audit of GNOME Session 50.0 found that only the
`ubuntu-desktop-session` leader/init-worker calls systemd Manager methods:
`GetUnit`, `StartUnit`, and `ResetFailed`, plus unit `Properties.Get` /
`GetAll` and `PropertiesChanged`. Snapd emits those rules only in that app's
profile. The `gnome-session-service` app uses only the broker API for
environment updates. Other session apps retain the shared interface for its
existing session AppArmor rules, but receive neither systemd Manager methods
nor shared-environment write permission.
