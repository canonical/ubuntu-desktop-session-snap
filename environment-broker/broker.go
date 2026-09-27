package main

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/godbus/dbus/v5"
)

const maxValueLength = 4096

var (
	envNamePattern    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	localePattern     = regexp.MustCompile(`^[A-Za-z0-9_.@:-]*$`)
	desktopPattern    = regexp.MustCompile(`^[A-Za-z0-9:;._-]+$`)
	displayPattern    = regexp.MustCompile(`^(?:[A-Za-z0-9.-]+)?:[0-9]+(?:\.[0-9]+)?$`)
	localeNamePattern = regexp.MustCompile(`^LC_[A-Z0-9_]+$`)
	waylandPattern    = regexp.MustCompile(`^wayland-[0-9]+$`)
	cursorPattern     = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

type environmentManager interface {
	CallerCredentials(dbus.Sender) (uint32, string, error)
	UnsetAndSetEnvironment(unset, set []string) error
	UpdateActivationEnvironment(map[string]string) error
}

type broker struct {
	mu            sync.Mutex
	manager       environmentManager
	uid           uint32
	snapVariables []string
}

func (b *broker) UpdateEnvironment(sender dbus.Sender, set map[string]string, unset []string) *dbus.Error {
	if err := b.authorizeCaller(sender); err != nil {
		return dbus.MakeFailedError(err)
	}
	if err := b.updateEnvironment(set, unset); err != nil {
		return dbus.MakeFailedError(err)
	}
	return nil
}

func (b *broker) authorizeCaller(sender dbus.Sender) error {
	uid, label, err := b.manager.CallerCredentials(sender)
	if err != nil {
		return fmt.Errorf("authenticate session environment caller: %w", err)
	}
	if uid != b.uid || appArmorProfile(label) != "snap.ubuntu-desktop-session.gnome-session-service" {
		return fmt.Errorf("unauthorized session environment caller %q with label %q", sender, label)
	}
	return nil
}

func appArmorProfile(label string) string {
	label = strings.TrimRight(label, "\x00")
	for _, mode := range []string{" (enforce)", " (complain)"} {
		label = strings.TrimSuffix(label, mode)
	}
	return label
}

func (b *broker) updateEnvironment(set map[string]string, unset []string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	accepted := make(map[string]string)
	unsetSet := make(map[string]struct{}, len(b.snapVariables)+len(unset))
	for _, name := range b.snapVariables {
		unsetSet[name] = struct{}{}
	}
	for name, value := range set {
		if !envNamePattern.MatchString(name) || !validText(value) {
			return fmt.Errorf("invalid environment entry %q", name)
		}
		if isSnapVariable(name) {
			unsetSet[name] = struct{}{}
			continue
		}
		if isForcedVariable(name) {
			continue
		}
		if !allowedVariable(name) {
			continue
		}
		if value == "" {
			unsetSet[name] = struct{}{}
			continue
		}
		if !validValue(name, value, b.uid) {
			return fmt.Errorf("invalid value for environment variable %s", name)
		}
		accepted[name] = value
	}

	for _, name := range unset {
		if !envNamePattern.MatchString(name) {
			return fmt.Errorf("invalid unset environment variable %q", name)
		}
		if isSnapVariable(name) {
			unsetSet[name] = struct{}{}
		} else if allowedVariable(name) && !isForcedVariable(name) {
			unsetSet[name] = struct{}{}
		}
	}

	for name := range set {
		if isSnapVariable(name) {
			unsetSet[name] = struct{}{}
		}
	}

	runtimeDir := fmt.Sprintf("/run/user/%d", b.uid)
	accepted["XDG_RUNTIME_DIR"] = runtimeDir
	accepted["XAUTHORITY"] = path.Join(runtimeDir, ".Xauthority")

	unsetNames := make([]string, 0, len(unsetSet))
	activation := make(map[string]string, len(accepted)+len(unsetSet))
	for name := range unsetSet {
		if _, overwritten := accepted[name]; overwritten {
			continue
		}
		unsetNames = append(unsetNames, name)
		activation[name] = ""
	}
	for name, value := range accepted {
		activation[name] = value
	}

	managerSet := make([]string, 0, len(accepted))
	acceptedNames := make([]string, 0, len(accepted))
	for name := range accepted {
		acceptedNames = append(acceptedNames, name)
	}
	sort.Strings(acceptedNames)
	for _, name := range acceptedNames {
		value := accepted[name]
		managerSet = append(managerSet, name+"="+value)
	}

	sort.Strings(unsetNames)
	if err := b.manager.UpdateActivationEnvironment(activation); err != nil {
		return fmt.Errorf("update D-Bus activation environment: %w", err)
	}
	// On this image, D-Bus activation updates also seed the user manager.
	// Apply the explicit manager update last so unset names stay unset there.
	if err := b.manager.UnsetAndSetEnvironment(unsetNames, managerSet); err != nil {
		return fmt.Errorf("update user manager environment: %w", err)
	}
	return nil
}

func snapEnvironmentVariableNames(environment []string) []string {
	names := make(map[string]struct{})
	for _, entry := range environment {
		name, _, ok := strings.Cut(entry, "=")
		if ok && isSnapVariable(name) {
			names[name] = struct{}{}
		}
	}
	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func allowedVariable(name string) bool {
	switch name {
	case "DISPLAY", "WAYLAND_DISPLAY", "XDG_SESSION_TYPE",
		"XDG_CURRENT_DESKTOP", "XDG_SESSION_DESKTOP",
		"XCURSOR_THEME", "XCURSOR_SIZE", "GNOME_SETUP_DISPLAY",
		"LANG", "LANGUAGE", "PATH", "XDG_DATA_DIRS":
		return true
	}
	return localeNamePattern.MatchString(name)
}

func validValue(name, value string, uid uint32) bool {
	switch {
	case name == "DISPLAY" || name == "GNOME_SETUP_DISPLAY":
		return displayPattern.MatchString(value)
	case name == "WAYLAND_DISPLAY":
		return validWaylandDisplay(value, uid)
	case name == "XDG_SESSION_TYPE":
		return value == "wayland" || value == "x11" || value == "tty"
	case name == "XDG_CURRENT_DESKTOP" || name == "XDG_SESSION_DESKTOP":
		return desktopPattern.MatchString(value)
	case name == "XCURSOR_THEME":
		return cursorPattern.MatchString(value)
	case name == "XCURSOR_SIZE":
		size, err := strconv.Atoi(value)
		return err == nil && size > 0 && size <= 1024
	case name == "PATH":
		return validPathList(value, map[string]bool{
			"/bin": true, "/sbin": true, "/usr/bin": true, "/usr/sbin": true,
			"/usr/local/bin": true, "/usr/local/sbin": true, "/snap/bin": true,
		})
	case name == "XDG_DATA_DIRS":
		allowed := map[string]bool{
			"/usr/share": true, "/usr/local/share": true,
			"/var/lib/snapd/desktop": true,
		}
		for _, dir := range strings.Split(value, ":") {
			if !allowed[dir] && !strings.HasPrefix(dir, "/snap/") {
				return false
			}
			if !cleanAbsolutePath(dir) {
				return false
			}
		}
		return value != ""
	case name == "LANG" || name == "LANGUAGE" || strings.HasPrefix(name, "LC_"):
		return localePattern.MatchString(value)
	default:
		return false
	}
}

func validPathList(value string, allowed map[string]bool) bool {
	if value == "" {
		return false
	}
	for _, entry := range strings.Split(value, ":") {
		if !allowed[entry] || !cleanAbsolutePath(entry) {
			return false
		}
	}
	return true
}

func validWaylandDisplay(value string, uid uint32) bool {
	if waylandPattern.MatchString(value) {
		return true
	}
	runtimePath := fmt.Sprintf("/run/user/%d", uid)
	return path.Dir(value) == runtimePath && waylandPattern.MatchString(path.Base(value))
}

func cleanAbsolutePath(value string) bool {
	return path.IsAbs(value) && path.Clean(value) == value &&
		!strings.Contains(value, "//")
}

func validText(value string) bool {
	if len(value) > maxValueLength || !utf8.ValidString(value) {
		return false
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return false
		}
	}
	return true
}

func isSnapVariable(name string) bool {
	return name == "SNAP" || strings.HasPrefix(name, "SNAP_")
}

func isForcedVariable(name string) bool {
	return name == "XDG_RUNTIME_DIR" || name == "XAUTHORITY"
}
