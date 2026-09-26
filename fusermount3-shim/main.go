// fusermount3-shim is a drop-in replacement for libfuse's fusermount3 that
// does not need any real privilege. Instead of calling mount(2) itself (which
// requires CAP_SYS_ADMIN and is impossible inside a strictly-confined snap on
// Ubuntu Core), it forwards the request to the privileged xdg-fuse-broker
// daemon over a UNIX socket, receives the live /dev/fuse fd via SCM_RIGHTS,
// and relays that fd back to libfuse over the _FUSE_COMMFD socketpair using
// the exact framing fusermount3 uses.
//
// It implements only the subset of fusermount3's CLI contract that libfuse
// actually depends on:
//
//	mount:        fusermount3 -o <opts> -- <mountpoint>   (with _FUSE_COMMFD set)
//	auto-unmount: fusermount3 --auto-unmount -- <mountpoint> (with _FUSE_COMMFD set)
//	unmount:      fusermount3 --unmount --quiet --lazy -- <mountpoint>
//
// The --auto-unmount form is a second invocation libfuse makes when the
// "auto_unmount" mount option is used (as xdg-document-portal does): it must
// block until the _FUSE_COMMFD socketpair is closed by the parent (i.e. the
// FUSE daemon died) and only then unmount.
//
// See libfuse util/fusermount.c (send_fd, wait_for_auto_unmount) and
// lib/mount.c (fuse_mount_fusermount / setup_auto_unmount / receive_fd) for
// the protocol this mirrors.
package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"syscall"
)

const (
	commfdEnv     = "_FUSE_COMMFD"
	brokerSockEnv = "XDG_FUSE_BROKER_SOCKET"
	defaultSocket = "/run/xdg-fuse-broker.sock"

	cmdMount   = 'M'
	cmdUnmount = 'U'

	statusOK    = '0'
	statusError = '1'

	// version reported for `fusermount3 --version`; libfuse only logs it.
	shimVersion = "3.16.2 (xdg-fuse-broker shim)"
)

var logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
	Level: slog.LevelInfo,
}))

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	opts, err := parseArgs(argv)
	if err != nil {
		logger.Error("argument error", "error", err)
		return 1
	}

	switch opts.mode {
	case modeVersion:
		fmt.Printf("fusermount3 version: %s\n", shimVersion)
		return 0
	case modeHelp:
		printHelp()
		return 0
	}

	uid := uint32(os.Getuid())
	subpath, err := subpathFromMountpoint(opts.mountpoint, uid)
	if err != nil {
		logger.Error("cannot derive broker subpath from mountpoint",
			"mountpoint", opts.mountpoint, "uid", uid, "error", err)
		return 1
	}

	sockPath := os.Getenv(brokerSockEnv)
	if sockPath == "" {
		sockPath = defaultSocket
	}

	conn, err := connectBroker(sockPath)
	if err != nil {
		logger.Error("cannot connect to broker", "socket", sockPath, "error", err)
		return 1
	}
	defer syscall.Close(conn)

	if opts.autoUnmount {
		return doAutoUnmount(conn, uid, subpath, opts)
	}
	if opts.unmount {
		return doUnmount(conn, uid, subpath, opts)
	}
	return doMount(conn, uid, subpath, opts)
}

// doAutoUnmount implements the `--auto-unmount` invocation: it does not mount
// anything, but blocks until the _FUSE_COMMFD socketpair is closed (the FUSE
// daemon has exited), then asks the broker to unmount. This mirrors libfuse's
// wait_for_auto_unmount path.
func doAutoUnmount(conn int, uid uint32, subpath string, opts *options) int {
	commfdStr := os.Getenv(commfdEnv)
	if commfdStr == "" {
		logger.Error("missing _FUSE_COMMFD: cannot wait for auto-unmount")
		return 1
	}
	commfd, err := strconv.Atoi(commfdStr)
	if err != nil {
		logger.Error("invalid _FUSE_COMMFD", "value", commfdStr, "error", err)
		return 1
	}

	logger.Info("auto-unmount: waiting for FUSE daemon to exit",
		"uid", uid, "subpath", subpath, "commfd", commfd)

	buf := make([]byte, 16)
	for {
		n, err := syscall.Read(commfd, buf)
		if n == 0 {
			break // EOF: parent closed the socketpair
		}
		if err != nil {
			if err == syscall.EINTR {
				continue
			}
			logger.Error("auto-unmount: error reading commfd",
				"commfd", commfd, "error", err)
			return 1
		}
	}

	logger.Info("auto-unmount: FUSE daemon exited, unmounting",
		"uid", uid, "subpath", subpath)
	return doUnmount(conn, uid, subpath, opts)
}

func doMount(conn int, uid uint32, subpath string, opts *options) int {
	commfdStr := os.Getenv(commfdEnv)
	if commfdStr == "" {
		logger.Error("missing _FUSE_COMMFD: old-style mounting is not supported")
		return 1
	}
	commfd, err := strconv.Atoi(commfdStr)
	if err != nil {
		logger.Error("invalid _FUSE_COMMFD", "value", commfdStr, "error", err)
		return 1
	}

	if err := sendRequest(conn, cmdMount, uid, subpath); err != nil {
		logger.Error("failed to send mount request", "error", err)
		return 1
	}

	fuseFd, err := recvFD(conn)
	if err != nil {
		logger.Error("failed to receive fuse fd from broker", "error", err)
		return 1
	}
	defer syscall.Close(fuseFd)

	if err := relayFD(commfd, fuseFd); err != nil {
		logger.Error("failed to relay fuse fd to libfuse",
			"commfd", commfd, "error", err)
		return 1
	}

	logger.Info("mounted via broker",
		"uid", uid, "subpath", subpath, "mountpoint", opts.mountpoint)
	return 0
}

func doUnmount(conn int, uid uint32, subpath string, opts *options) int {
	if err := sendRequest(conn, cmdUnmount, uid, subpath); err != nil {
		logger.Error("failed to send unmount request", "error", err)
		return 1
	}
	if err := recvStatus(conn); err != nil {
		// fusermount3 with --quiet suppresses unmount errors; libfuse passes
		// --quiet on the unmount path, so honour it.
		if opts.quiet {
			logger.Info("unmount reported error (suppressed by --quiet)",
				"subpath", subpath, "error", err)
			return 0
		}
		logger.Error("unmount failed", "subpath", subpath, "error", err)
		return 1
	}
	logger.Info("unmounted via broker", "uid", uid, "subpath", subpath)
	return 0
}

// --- argument parsing -------------------------------------------------------

type parseMode int

const (
	modeMount parseMode = iota
	modeVersion
	modeHelp
)

type options struct {
	mode        parseMode
	unmount     bool
	autoUnmount bool
	lazy        bool
	quiet       bool
	mountpoint  string
}

// parseArgs mirrors the subset of fusermount3's getopt_long handling that
// libfuse relies on. Unknown -o options are accepted and ignored (the broker
// chooses the real mount options).
func parseArgs(argv []string) (*options, error) {
	opts := &options{mode: modeMount}
	var positional []string
	seenDashDash := false

	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if seenDashDash {
			positional = append(positional, arg)
			continue
		}
		switch {
		case arg == "--":
			seenDashDash = true
		case arg == "-u" || arg == "--unmount":
			opts.unmount = true
		case arg == "--auto-unmount":
			// Internal flag used by libfuse's setup_auto_unmount; no short form.
			opts.autoUnmount = true
		case arg == "-z" || arg == "--lazy":
			opts.lazy = true
		case arg == "-q" || arg == "--quiet":
			opts.quiet = true
		case arg == "-V" || arg == "--version":
			opts.mode = modeVersion
			return opts, nil
		case arg == "-h" || arg == "--help":
			opts.mode = modeHelp
			return opts, nil
		case arg == "-o":
			// -o takes a value; consume it (ignored).
			if i+1 >= len(argv) {
				return nil, errors.New("-o requires an argument")
			}
			i++
		case strings.HasPrefix(arg, "-o"):
			// -o<value> form; ignored.
		case strings.HasPrefix(arg, "-") && arg != "-":
			// Unknown flag: fusermount3 would reject it. Be strict so we
			// don't silently mis-handle a flag libfuse depends on.
			return nil, fmt.Errorf("unknown option %q", arg)
		default:
			positional = append(positional, arg)
		}
	}

	if opts.mode != modeMount {
		return opts, nil
	}

	if len(positional) == 0 {
		return nil, errors.New("missing mountpoint argument")
	}
	if len(positional) > 1 {
		return nil, errors.New("extra arguments after the mountpoint")
	}
	opts.mountpoint = positional[0]
	return opts, nil
}

func printHelp() {
	fmt.Print(`fusermount3 (xdg-fuse-broker shim)
Usage: fusermount3 [OPTIONS] MOUNTPOINT
  -h  --help      print help
  -V  --version   print version
  -o OPTION       mount options (accepted, ignored)
  -u  --unmount   unmount
  -q  --quiet     quiet
  -z  --lazy      lazy unmount
      --auto-unmount  wait for the FUSE daemon to exit, then unmount
`)
}

// --- broker protocol --------------------------------------------------------

// subpathFromMountpoint converts an absolute mountpoint under
// /run/user/<uid>/ into the relative subpath the broker expects. The broker
// re-validates this against its allowlist and reconstructs the real path
// server-side, so the client can never influence the /run/user/<uid> prefix.
func subpathFromMountpoint(mountpoint string, uid uint32) (string, error) {
	prefix := fmt.Sprintf("/run/user/%d/", uid)
	if !strings.HasPrefix(mountpoint, prefix) {
		return "", fmt.Errorf("mountpoint %q is not under %q", mountpoint, prefix)
	}
	sub := strings.TrimPrefix(mountpoint, prefix)
	sub = strings.TrimSuffix(sub, "/")
	if sub == "" {
		return "", fmt.Errorf("mountpoint %q has empty subpath", mountpoint)
	}
	return sub, nil
}

func connectBroker(sockPath string) (int, error) {
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return -1, err
	}
	if err := syscall.Connect(fd, &syscall.SockaddrUnix{Name: sockPath}); err != nil {
		syscall.Close(fd)
		return -1, err
	}
	return fd, nil
}

// sendRequest writes the broker request frame:
//
//	[0]      command byte ('M'/'U')
//	[1:5]    requested uid (uint32, big endian)
//	[5:7]    subpath length (uint16, big endian)
//	[7:7+n]  subpath
func sendRequest(conn int, cmd byte, uid uint32, subpath string) error {
	req := make([]byte, 7+len(subpath))
	req[0] = cmd
	binary.BigEndian.PutUint32(req[1:5], uid)
	binary.BigEndian.PutUint16(req[5:7], uint16(len(subpath)))
	copy(req[7:], subpath)
	_, err := syscall.Write(conn, req)
	return err
}

// recvFD reads the broker's reply and, on success, returns the received fd.
func recvFD(conn int) (int, error) {
	buf := make([]byte, 4096)
	oob := make([]byte, syscall.CmsgSpace(4))
	n, oobn, _, _, err := syscall.Recvmsg(conn, buf, oob, 0)
	if err != nil {
		return -1, err
	}
	if n == 0 {
		return -1, errors.New("broker closed connection with no reply")
	}
	if buf[0] == statusError {
		return -1, fmt.Errorf("broker error: %s", string(buf[1:n]))
	}
	msgs, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return -1, fmt.Errorf("parse control message: %w", err)
	}
	if len(msgs) == 0 {
		return -1, errors.New("broker sent no control message")
	}
	fds, err := syscall.ParseUnixRights(&msgs[0])
	if err != nil {
		return -1, fmt.Errorf("parse unix rights: %w", err)
	}
	if len(fds) == 0 {
		return -1, errors.New("broker sent no file descriptor")
	}
	return fds[0], nil
}

// recvStatus reads a status-only reply (used for unmount).
func recvStatus(conn int) error {
	buf := make([]byte, 4096)
	n, _, _, _, err := syscall.Recvmsg(conn, buf, nil, 0)
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("broker closed connection with no reply")
	}
	if buf[0] == statusError {
		return fmt.Errorf("broker error: %s", string(buf[1:n]))
	}
	return nil
}

// relayFD sends the fuse fd to libfuse over the _FUSE_COMMFD socketpair using
// the exact framing fusermount3 uses: a single zero status byte plus the fd as
// SCM_RIGHTS ancillary data in one sendmsg.
func relayFD(commfd, fuseFd int) error {
	oob := syscall.UnixRights(fuseFd)
	err := syscall.Sendmsg(commfd, []byte{0}, oob, nil, 0)
	if err == syscall.EINTR {
		err = syscall.Sendmsg(commfd, []byte{0}, oob, nil, 0)
	}
	return err
}
