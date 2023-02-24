package app

import (
	"context"
	"errors"

	"github.com/godbus/dbus/v5"
)

type MockAuthorizer struct {
	RejectAuth bool
}

type MockProxy struct {
	ApplyError bool
}

func (m *MockAuthorizer) IsSenderAllowed(ctx context.Context, action string, sender dbus.Sender) (err error) {
	if m.RejectAuth {
		err = errors.New("authorization rejected")
	}

	return err
}

func (m *MockProxy) Apply(ctx context.Context, _, _, _, _, _, _ string) error {
	if m.ApplyError {
		return errors.New("proxy apply error")
	}
	return nil
}

func WithAuthorizer(a authorizerer) func(*options) {
	return func(o *options) {
		o.authorizer = a
	}
}

func WithProxy(p proxyApplier) func(*options) {
	return func(o *options) {
		o.proxy = p
	}
}
