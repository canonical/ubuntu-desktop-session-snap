package main

import (
	"fmt"
	"log"
	"os"

	"github.com/godbus/dbus/v5"
)

const (
	busName       = "org.ubuntu.SessionEnvironmentBroker"
	objectPath    = dbus.ObjectPath("/org/ubuntu/SessionEnvironmentBroker")
	interfaceName = "org.ubuntu.SessionEnvironmentBroker"
)

// snapRuntimeDir returns the snap-private runtime dir this daemon runs in
// (/run/user/<uid>/snap.<instance>), which is also where the session's
// confined compositor creates its wayland socket. Returns "" when the
// environment does not look like that dir, disabling the WAYLAND_DISPLAY
// absolutization.
func snapRuntimeDir(uid int) string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	instance := os.Getenv("SNAP_INSTANCE_NAME")
	if runtimeDir == "" || instance == "" {
		return ""
	}
	if runtimeDir != fmt.Sprintf("/run/user/%d/snap.%s", uid, instance) {
		return ""
	}
	return runtimeDir
}

func main() {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		log.Printf("connect to session bus: %v", err)
		os.Exit(1)
	}
	defer conn.Close()

	b := &broker{
		manager:        sessionBusManager{conn: conn},
		uid:            uint32(os.Getuid()),
		snapVariables:  snapEnvironmentVariableNames(os.Environ()),
		snapRuntimeDir: snapRuntimeDir(os.Getuid()),
	}
	if err := conn.Export(b, objectPath, interfaceName); err != nil {
		log.Printf("export environment broker: %v", err)
		os.Exit(1)
	}
	if _, err := conn.RequestName(busName, dbus.NameFlagDoNotQueue); err != nil {
		log.Printf("own environment broker name: %v", err)
		os.Exit(1)
	}

	select {}
}

type sessionBusManager struct {
	conn *dbus.Conn
}

func (m sessionBusManager) UnsetAndSetEnvironment(unset, set []string) error {
	return m.conn.Object("org.freedesktop.systemd1", "/org/freedesktop/systemd1").
		Call("org.freedesktop.systemd1.Manager.UnsetAndSetEnvironment", 0, unset, set).Err
}

func (m sessionBusManager) CallerCredentials(sender dbus.Sender) (uint32, string, error) {
	var credentials map[string]dbus.Variant
	err := m.conn.Object("org.freedesktop.DBus", "/org/freedesktop/DBus").
		Call("org.freedesktop.DBus.GetConnectionCredentials", 0, string(sender)).
		Store(&credentials)
	if err != nil {
		return 0, "", err
	}

	uidVariant, ok := credentials["UnixUserID"]
	if !ok {
		return 0, "", fmt.Errorf("D-Bus credentials do not include UnixUserID")
	}
	uid, ok := uidVariant.Value().(uint32)
	if !ok {
		return 0, "", fmt.Errorf("D-Bus UnixUserID has unexpected type %T", uidVariant.Value())
	}

	labelVariant, ok := credentials["LinuxSecurityLabel"]
	if !ok {
		return 0, "", fmt.Errorf("D-Bus credentials do not include LinuxSecurityLabel")
	}
	label, ok := labelVariant.Value().([]byte)
	if !ok {
		return 0, "", fmt.Errorf("D-Bus LinuxSecurityLabel has unexpected type %T", labelVariant.Value())
	}
	return uid, string(label), nil
}

func (m sessionBusManager) UpdateActivationEnvironment(environment map[string]string) error {
	return m.conn.Object("org.freedesktop.DBus", "/org/freedesktop/DBus").
		Call("org.freedesktop.DBus.UpdateActivationEnvironment", 0, environment).Err
}
