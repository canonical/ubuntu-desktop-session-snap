const { Gio, GLib } = imports.gi;
const connection = Gio.bus_get_sync(Gio.BusType.SESSION, null);
const started = GLib.get_monotonic_time();
const timeout = 15 * 1000000;
let ready = false;

while (GLib.get_monotonic_time() - started < timeout) {
  const reply = connection.call_sync(
    "org.freedesktop.DBus",
    "/org/freedesktop/DBus",
    "org.freedesktop.DBus",
    "NameHasOwner",
    new GLib.Variant("(s)", ["org.gnome.Mutter.ServiceChannel"]),
    new GLib.VariantType("(b)"),
    Gio.DBusCallFlags.NONE,
    1000,
    null,
  );
  if (reply.get_child_value(0).get_boolean()) {
    ready = true;
    break;
  }
  GLib.usleep(250000);
}

if (!ready) {
  printerr("Timed out waiting for org.gnome.Mutter.ServiceChannel; continuing without display-dependent portals");
}
