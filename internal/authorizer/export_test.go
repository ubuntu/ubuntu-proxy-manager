package authorizer

import (
	"errors"

	"github.com/godbus/dbus/v5"
)

func WithAuthority(c caller) func(*options) {
	return func(o *options) {
		o.authority = c
	}
}

func WithCredLookup(c caller) func(*options) {
	return func(o *options) {
		o.credLookup = c
	}
}

func WithRoot(root string) func(*options) {
	return func(o *options) {
		o.root = root
	}
}

type PolkitObjMock struct {
	IsAuthorized    bool
	WantPolkitError bool

	actionRequested string
}

func (d *PolkitObjMock) Call(method string, flags dbus.Flags, args ...interface{}) *dbus.Call {
	var errPolkit error

	content, ok := args[1].(string)
	if !ok {
		panic("Expected string as second argument")
	}

	d.actionRequested = content

	if d.WantPolkitError {
		errPolkit = errors.New("Polkit error")
	}

	return &dbus.Call{
		Err: errPolkit,
		Body: []interface{}{
			[]interface{}{
				d.IsAuthorized,
				true,
				map[string]string{
					"polkit.retains_authorization_after_challenge": "true",
					"polkit.dismissed": "true",
				},
			},
		},
	}
}

type CredsObjMock struct {
	UID             any
	PID             any
	WantLookupError bool
}

func (d *CredsObjMock) Call(method string, flags dbus.Flags, args ...interface{}) *dbus.Call {
	var errCredsLookup error

	if d.WantLookupError {
		errCredsLookup = errors.New("Credentials lookup error")
	}

	return &dbus.Call{
		Err: errCredsLookup,
		Body: []interface{}{
			map[string]dbus.Variant{
				"UnixUserID": dbus.MakeVariant(d.UID),
				"ProcessID":  dbus.MakeVariant(d.PID),
			}},
	}
}
