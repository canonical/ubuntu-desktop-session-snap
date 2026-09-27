package main

import (
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/godbus/dbus/v5"
)

type fakeEnvironmentManager struct {
	unset            []string
	set              []string
	activation       map[string]string
	calls            []string
	callerUID        uint32
	callerLabel      string
	credentialsErr   error
	managerUpdateErr error
	activationErr    error
}

func (m *fakeEnvironmentManager) CallerCredentials(dbus.Sender) (uint32, string, error) {
	return m.callerUID, m.callerLabel, m.credentialsErr
}

func (m *fakeEnvironmentManager) UnsetAndSetEnvironment(unset, set []string) error {
	m.calls = append(m.calls, "systemd")
	m.unset = append([]string(nil), unset...)
	m.set = append([]string(nil), set...)
	return m.managerUpdateErr
}

func (m *fakeEnvironmentManager) UpdateActivationEnvironment(environment map[string]string) error {
	m.calls = append(m.calls, "activation")
	m.activation = make(map[string]string, len(environment))
	for name, value := range environment {
		m.activation[name] = value
	}
	return m.activationErr
}

func TestUpdateEnvironmentFiltersAndRewrites(t *testing.T) {
	manager := &fakeEnvironmentManager{}
	b := &broker{
		manager:       manager,
		uid:           1000,
		snapVariables: []string{"SNAP", "SNAP_REVISION"},
	}

	err := b.updateEnvironment(map[string]string{
		"DISPLAY":            ":0",
		"WAYLAND_DISPLAY":    "/run/user/1000/wayland-0",
		"XDG_SESSION_TYPE":   "wayland",
		"LANG":               "en_US.UTF-8",
		"SNAP":               "/snap/attacker/1",
		"SNAP_INSTANCE_NAME": "attacker",
		"LD_PRELOAD":         "/tmp/evil.so",
		"XDG_RUNTIME_DIR":    "/run/user/1000/snap.untrusted",
		"XAUTHORITY":         "/tmp/untrusted.xauth",
		"XDG_DATA_HOME":      "/home/test/snap/private",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	wantUnset := []string{"SNAP", "SNAP_INSTANCE_NAME", "SNAP_REVISION"}
	sort.Strings(manager.unset)
	if !reflect.DeepEqual(manager.unset, wantUnset) {
		t.Fatalf("unset environment = %v, want %v", manager.unset, wantUnset)
	}

	wantSet := []string{
		"DISPLAY=:0",
		"LANG=en_US.UTF-8",
		"WAYLAND_DISPLAY=/run/user/1000/wayland-0",
		"XDG_RUNTIME_DIR=/run/user/1000",
		"XAUTHORITY=/run/user/1000/.Xauthority",
		"XDG_SESSION_TYPE=wayland",
	}
	sort.Strings(wantSet)
	sort.Strings(manager.set)
	if !reflect.DeepEqual(manager.set, wantSet) {
		t.Fatalf("systemd environment = %v, want %v", manager.set, wantSet)
	}

	wantActivation := map[string]string{
		"DISPLAY":            ":0",
		"LANG":               "en_US.UTF-8",
		"WAYLAND_DISPLAY":    "/run/user/1000/wayland-0",
		"XDG_RUNTIME_DIR":    "/run/user/1000",
		"XAUTHORITY":         "/run/user/1000/.Xauthority",
		"XDG_SESSION_TYPE":   "wayland",
		"SNAP":               "",
		"SNAP_REVISION":      "",
		"SNAP_INSTANCE_NAME": "",
	}
	if !reflect.DeepEqual(manager.activation, wantActivation) {
		t.Fatalf("activation environment = %#v, want %#v", manager.activation, wantActivation)
	}
	if !reflect.DeepEqual(manager.calls, []string{"activation", "systemd"}) {
		t.Fatalf("environment update order = %v, want [activation systemd]", manager.calls)
	}
}

func TestUpdateEnvironmentClearsRequestedAllowlistedVariables(t *testing.T) {
	manager := &fakeEnvironmentManager{}
	b := &broker{manager: manager, uid: 1000}

	if err := b.updateEnvironment(nil, []string{"DISPLAY", "XDG_RUNTIME_DIR", "LD_PRELOAD"}); err != nil {
		t.Fatal(err)
	}

	sort.Strings(manager.unset)
	if !reflect.DeepEqual(manager.unset, []string{"DISPLAY"}) {
		t.Fatalf("unset environment = %v, want [DISPLAY]", manager.unset)
	}
	if manager.activation["DISPLAY"] != "" {
		t.Fatalf("activation DISPLAY = %q, want empty", manager.activation["DISPLAY"])
	}
	if manager.activation["XDG_RUNTIME_DIR"] != "/run/user/1000" ||
		manager.activation["XAUTHORITY"] != "/run/user/1000/.Xauthority" {
		t.Fatalf("forced paths missing from activation environment: %#v", manager.activation)
	}
}

func TestUpdateEnvironmentTreatsEmptyValuesAsUnsets(t *testing.T) {
	manager := &fakeEnvironmentManager{}
	b := &broker{manager: manager, uid: 1000}

	if err := b.updateEnvironment(map[string]string{
		"GNOME_SETUP_DISPLAY": "",
		"DISPLAY":             "",
	}, nil); err != nil {
		t.Fatal(err)
	}

	sort.Strings(manager.unset)
	if !reflect.DeepEqual(manager.unset, []string{"DISPLAY", "GNOME_SETUP_DISPLAY"}) {
		t.Fatalf("unset environment = %v, want [DISPLAY GNOME_SETUP_DISPLAY]", manager.unset)
	}
	if manager.activation["DISPLAY"] != "" || manager.activation["GNOME_SETUP_DISPLAY"] != "" {
		t.Fatalf("activation variables were not cleared: %#v", manager.activation)
	}
	if contains(manager.set, "DISPLAY=") || contains(manager.set, "GNOME_SETUP_DISPLAY=") {
		t.Fatalf("empty values were set in systemd: %v", manager.set)
	}
}

func TestUpdateEnvironmentRejectsMalformedInputBeforeWriting(t *testing.T) {
	manager := &fakeEnvironmentManager{}
	b := &broker{manager: manager, uid: 1000}

	for _, test := range []struct {
		name  string
		value string
	}{
		{name: "DISPLAY", value: ":0\nLD_PRELOAD=/tmp/evil.so"},
		{name: "XCURSOR_SIZE", value: "999999"},
		{name: "PATH", value: "/usr/bin:/run/user/1000/snap.app/bin"},
		{name: "WAYLAND_DISPLAY", value: "/run/user/1000/snap.app/wayland-0"},
		{name: "XDG_DATA_DIRS", value: "/usr/share:/run/user/1000/snap.app/share"},
		{name: "bad=name", value: "value"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := b.updateEnvironment(map[string]string{test.name: test.value}, nil)
			if err == nil {
				t.Fatal("expected invalid input to be rejected")
			}
			if manager.set != nil || manager.activation != nil {
				t.Fatalf("invalid input caused a partial update: set=%v activation=%v",
					manager.set, manager.activation)
			}
		})
	}
}

func TestUpdateEnvironmentPropagatesManagerFailures(t *testing.T) {
	t.Run("manager update", func(t *testing.T) {
		manager := &fakeEnvironmentManager{managerUpdateErr: errors.New("update failed")}
		err := (&broker{manager: manager, uid: 1000}).updateEnvironment(nil, nil)
		if err == nil {
			t.Fatal("expected manager update failure")
		}
		if manager.activation == nil {
			t.Fatal("D-Bus environment was not updated before manager failure")
		}
	})

	t.Run("activation update", func(t *testing.T) {
		manager := &fakeEnvironmentManager{
			activationErr: errors.New("activation failed"),
		}
		err := (&broker{manager: manager, uid: 1000}).updateEnvironment(
			map[string]string{"DISPLAY": ":0"},
			nil,
		)
		if err == nil {
			t.Fatal("expected activation environment failure")
		}
		if manager.set != nil {
			t.Fatalf("systemd environment updated after D-Bus failure: %v", manager.set)
		}
	})
}

func TestRepeatedEnvironmentUpdates(t *testing.T) {
	manager := &fakeEnvironmentManager{}
	b := &broker{manager: manager, uid: 1000}

	for _, display := range []string{":0", ":1"} {
		if err := b.updateEnvironment(map[string]string{"DISPLAY": display}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if manager.activation["DISPLAY"] != ":1" {
		t.Fatalf("latest DISPLAY = %q, want :1", manager.activation["DISPLAY"])
	}
	if !contains(manager.set, "DISPLAY=:1") {
		t.Fatalf("latest systemd environment does not contain DISPLAY=:1: %v", manager.set)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestSnapEnvironmentVariableNames(t *testing.T) {
	got := snapEnvironmentVariableNames([]string{
		"SNAP=/snap/example/1",
		"SNAP_REVISION=1",
		"DISPLAY=:0",
		"SNAP=/snap/example/1",
		"SNAP_USER_DATA=/home/user/snap/example/1",
	})
	want := []string{"SNAP", "SNAP_REVISION", "SNAP_USER_DATA"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snap environment variable names = %v, want %v", got, want)
	}
}

func TestUpdateEnvironmentAuthenticatesSessionManager(t *testing.T) {
	for _, test := range []struct {
		name           string
		uid            uint32
		label          string
		credentialsErr error
		wantError      bool
	}{
		{
			name:      "intended session manager",
			uid:       1000,
			label:     "snap.ubuntu-desktop-session.gnome-session-service (enforce)\x00",
			wantError: false,
		},
		{
			name:      "other app in same snap",
			uid:       1000,
			label:     "snap.ubuntu-desktop-session.dconf-service (enforce)",
			wantError: true,
		},
		{
			name:      "different user",
			uid:       1001,
			label:     "snap.ubuntu-desktop-session.gnome-session-service (enforce)",
			wantError: true,
		},
		{
			name:           "credentials unavailable",
			credentialsErr: errors.New("credentials unavailable"),
			wantError:      true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager := &fakeEnvironmentManager{
				callerUID:      test.uid,
				callerLabel:    test.label,
				credentialsErr: test.credentialsErr,
			}
			b := &broker{
				manager: manager,
				uid:     1000,
			}

			err := b.UpdateEnvironment(
				dbus.Sender(":1.23"),
				map[string]string{"DISPLAY": ":0"},
				nil,
			)
			if (err != nil) != test.wantError {
				t.Fatalf("UpdateEnvironment error = %v, want error %v", err, test.wantError)
			}
			if test.wantError && (manager.set != nil || manager.activation != nil) {
				t.Fatalf("unauthorized call changed environment: set=%v activation=%v",
					manager.set, manager.activation)
			}
			if !test.wantError && manager.activation["DISPLAY"] != ":0" {
				t.Fatalf("authorized call did not update activation environment: %#v", manager.activation)
			}
		})
	}
}
