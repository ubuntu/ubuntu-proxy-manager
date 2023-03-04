package app

import (
	"errors"

	"github.com/godbus/dbus/v5"
)

// MockAuthorizer is a mock authorizer.
type MockAuthorizer struct {
	RejectAuth bool
}

// MockProxy is a mock proxy.
type MockProxy struct {
	ApplyError bool
}

// IsSenderAllowed is a mock implementation of authorizerer, returning an error if requested in the mock.
func (m *MockAuthorizer) IsSenderAllowed(action string, sender dbus.Sender) (err error) {
	if m.RejectAuth {
		err = errors.New("authorization rejected")
	}

	return err
}

// Apply is a mock implementation of proxier, returning an error if requested in the mock.
func (m *MockProxy) Apply(_, _, _, _, _, _ string) error {
	if m.ApplyError {
		return errors.New("proxy apply error")
	}
	return nil
}

// WithAuthorizer overrides the default authorizer implementation.
func WithAuthorizer(a authorizerer) func(*options) {
	return func(o *options) {
		o.authorizer = a
	}
}

// WithProxy overrides the default proxy applier implementation.
func WithProxy(p proxyApplier) func(*options) {
	return func(o *options) {
		o.proxy = p
	}
}
