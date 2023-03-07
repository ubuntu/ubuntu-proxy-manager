// Package authorizer deals client authorization based on a definite set of polkit actions.
// The client UID and PID are obtained via the D-Bus sender passed to the authorizing method.
package authorizer

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/godbus/dbus/v5"
	log "github.com/sirupsen/logrus"
	"github.com/ubuntu/decorate"
)

type caller interface {
	Call(method string, flags dbus.Flags, args ...interface{}) *dbus.Call
}

type options struct {
	authority  caller
	credLookup caller

	root string
}

type option func(*options)

// Authorizer is an abstraction of polkit authorization and D-Bus credential
// lookup.
type Authorizer struct {
	authority   caller
	credsLookup caller
	root        string
}

type polkitCheckFlags uint32

const (
	// checkAllowInteraction is a polkit flag indicating that if the subject can
	// obtain the authorization through authentication, and an authentication
	// agent is available, it will attempt to do so.
	//
	// This means that the CheckAuthorization() method will block while the user
	// is being asked to authenticate.
	checkAllowInteraction polkitCheckFlags = 0x01
)

type polkitAuthSubject struct {
	Kind    string
	Details map[string]dbus.Variant
}

type polkitAuthResult struct {
	IsAuthorized bool
	IsChallenge  bool
	Details      map[string]string
}

// New returns a new authorizer.
func New(bus *dbus.Conn, args ...option) *Authorizer {
	authority := bus.Object("org.freedesktop.PolicyKit1",
		"/org/freedesktop/PolicyKit1/Authority")

	credsLookup := bus.Object("org.freedesktop.DBus",
		"/org/freedesktop/DBus")

	opts := options{
		authority:  authority,
		credLookup: credsLookup,
		root:       "/",
	}

	// Apply given options
	for _, f := range args {
		f(&opts)
	}

	return &Authorizer{
		authority:   opts.authority,
		credsLookup: opts.credLookup,
		root:        opts.root,
	}
}

// CheckSenderAllowed returns nil if the user is allowed to perform a given operation.
// Based on the D-Bus sender it will query the user's credentials and then
// attempt to authorize the action using polkit.
func (a Authorizer) CheckSenderAllowed(action string, sender dbus.Sender) (err error) {
	log.Debugf("Check if sender %s is allowed to perform action %q", sender, action)
	defer decorate.OnError(&err, "permission denied")

	credsResult := make(map[string]dbus.Variant)
	if err = a.credsLookup.Call("org.freedesktop.DBus.GetConnectionCredentials", 0, string(sender)).Store(&credsResult); err != nil {
		return err
	}

	var uid, pid uint32
	uid, ok := credsResult["UnixUserID"].Value().(uint32)
	if !ok {
		return errors.New("can't get uid from dbus credentials")
	}
	pid, ok = credsResult["ProcessID"].Value().(uint32)
	if !ok {
		return errors.New("can't get pid from dbus credentials")
	}

	return a.isAllowed(action, pid, uid)
}

// isAllowed returns nil if the given uid/pid are allowed to perform the given action.
func (a Authorizer) isAllowed(action string, pid uint32, uid uint32) (err error) {
	if uid == 0 {
		log.Debug("Authorized as being administrator")
		return nil
	}

	f, err := os.Open(filepath.Join(a.root, fmt.Sprintf("proc/%d/stat", pid)))
	if err != nil {
		return fmt.Errorf("couldn't open stat file for process: %w", err)
	}
	defer func() { _ = f.Close() }()

	startTime, err := getStartTimeFromReader(f)
	if err != nil {
		return err
	}

	subject := polkitAuthSubject{
		Kind: "unix-process",
		Details: map[string]dbus.Variant{
			"pid":        dbus.MakeVariant(pid),
			"start-time": dbus.MakeVariant(startTime),
			"uid":        dbus.MakeVariant(uid),
		},
	}

	var result polkitAuthResult
	var details map[string]string
	err = a.authority.Call(
		"org.freedesktop.PolicyKit1.Authority.CheckAuthorization", dbus.FlagAllowInteractiveAuthorization,
		subject, action, details, checkAllowInteraction, "").Store(&result)
	if err != nil {
		return fmt.Errorf("call to polkit failed: %w", err)
	}
	log.Debugf("Polkit call result, authorized: %t", result.IsAuthorized)

	if !result.IsAuthorized {
		return errors.New("polkit denied access")
	}
	return nil
}

// getStartTimeFromReader determines the start time from a process stat file content
//
// The implementation is intended to be compatible with polkit:
//
//	https://cgit.freedesktop.org/polkit/tree/src/polkit/polkitunixprocess.c
func getStartTimeFromReader(r io.Reader) (t uint64, err error) {
	defer decorate.OnError(&err, "can't determine start time of client process")

	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	contents := string(data)

	// start time is the token at index 19 after the '(process
	// name)' entry - since only this field can contain the ')'
	// character, search backwards for this to avoid malicious
	// processes trying to fool us
	//
	// See proc(5) man page for a description of the
	// /proc/[pid]/stat file format and the meaning of the
	// starttime field.
	idx := strings.IndexByte(contents, ')')
	if idx < 0 {
		return 0, errors.New("parsing error: missing )")
	}
	idx += 2 // skip ") "
	if idx > len(contents) {
		return 0, errors.New("parsing error: ) at the end")
	}
	tokens := strings.Split(contents[idx:], " ")
	if len(tokens) < 20 {
		return 0, errors.New("parsing error: less fields than required")
	}
	v, err := strconv.ParseUint(tokens[19], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing error: %w", err)
	}
	return v, nil
}
