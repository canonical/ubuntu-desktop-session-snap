package main

import "testing"

func TestParseArgsMount(t *testing.T) {
	opts, err := parseArgs([]string{"-o", "subtype=portal,fsname=portal,auto_unmount", "--", "/run/user/1000/snap.ubuntu-desktop-session/doc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.mode != modeMount {
		t.Fatalf("mode = %v, want modeMount", opts.mode)
	}
	if opts.unmount {
		t.Fatalf("unmount = true, want false")
	}
	if opts.mountpoint != "/run/user/1000/snap.ubuntu-desktop-session/doc" {
		t.Fatalf("mountpoint = %q", opts.mountpoint)
	}
}

func TestParseArgsUnmount(t *testing.T) {
	opts, err := parseArgs([]string{"--unmount", "--quiet", "--lazy", "--", "/run/user/1000/snap.ubuntu-desktop-session/doc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.unmount || !opts.quiet || !opts.lazy {
		t.Fatalf("flags not parsed: %+v", opts)
	}
	if opts.mountpoint != "/run/user/1000/snap.ubuntu-desktop-session/doc" {
		t.Fatalf("mountpoint = %q", opts.mountpoint)
	}
}

func TestParseArgsShortUnmount(t *testing.T) {
	opts, err := parseArgs([]string{"-u", "-q", "-z", "--", "/run/user/1000/x/doc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.unmount || !opts.quiet || !opts.lazy {
		t.Fatalf("flags not parsed: %+v", opts)
	}
}

func TestParseArgsAutoUnmount(t *testing.T) {
	opts, err := parseArgs([]string{"--auto-unmount", "--", "/run/user/1000/snap.ubuntu-desktop-session/doc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.autoUnmount {
		t.Fatalf("autoUnmount = false, want true")
	}
	if opts.unmount {
		t.Fatalf("unmount = true, want false")
	}
	if opts.mountpoint != "/run/user/1000/snap.ubuntu-desktop-session/doc" {
		t.Fatalf("mountpoint = %q", opts.mountpoint)
	}
}

func TestParseArgsVersion(t *testing.T) {
	opts, err := parseArgs([]string{"--version"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.mode != modeVersion {
		t.Fatalf("mode = %v, want modeVersion", opts.mode)
	}
}

func TestParseArgsErrors(t *testing.T) {
	cases := [][]string{
		{},                      // missing mountpoint
		{"--", "/a", "/b"},      // extra args
		{"-o"},                  // -o without value
		{"--bogus", "--", "/a"}, // unknown flag
	}
	for _, c := range cases {
		if _, err := parseArgs(c); err == nil {
			t.Errorf("parseArgs(%v) = nil error, want error", c)
		}
	}
}

func TestSubpathFromMountpoint(t *testing.T) {
	got, err := subpathFromMountpoint("/run/user/1000/snap.ubuntu-desktop-session/doc", 1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "snap.ubuntu-desktop-session/doc" {
		t.Fatalf("subpath = %q", got)
	}

	// trailing slash tolerated
	got, err = subpathFromMountpoint("/run/user/1000/snap.ubuntu-desktop-session/doc/", 1000)
	if err != nil || got != "snap.ubuntu-desktop-session/doc" {
		t.Fatalf("trailing slash: got %q err %v", got, err)
	}
}

func TestSubpathFromMountpointRejects(t *testing.T) {
	cases := []struct {
		mp  string
		uid uint32
	}{
		{"/run/user/1001/snap.ubuntu-desktop-session/doc", 1000}, // wrong uid
		{"/tmp/doc", 1000},        // outside /run/user
		{"/run/user/1000/", 1000}, // empty subpath
		{"/run/user/1000", 1000},  // no trailing slash
	}
	for _, c := range cases {
		if _, err := subpathFromMountpoint(c.mp, c.uid); err == nil {
			t.Errorf("subpathFromMountpoint(%q, %d) = nil error, want error", c.mp, c.uid)
		}
	}
}
