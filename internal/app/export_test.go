package app

import (
	"context"
	"errors"

	"github.com/godbus/dbus/v5"
)

type MockAuthorizer struct {
	RejectAuth bool
}

func (m *MockAuthorizer) IsSenderAllowed(ctx context.Context, action string, sender dbus.Sender) (err error) {
	if m.RejectAuth {
		err = errors.New("authorization rejected")
	}

	return err
}

func WithAuthorizer(a authorizerer) func(*options) {
	return func(o *options) {
		o.authorizer = a
	}
}
