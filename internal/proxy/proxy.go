// Package proxy sets the proxy configuration for the system.
package proxy

import (
	"context"

	log "github.com/sirupsen/logrus"
)

// Proxy represents a proxy manager.
type Proxy struct{}

// DryRun is a context key to indicate that we should not apply the proxy
// settings. It is intended to be used in tests only.
var DryRun = struct{}{}

// New returns a new instance of a proxy manager.
func New(ctx context.Context, http, https, ftp, socks, no, mode string) (*Proxy, error) {
	return &Proxy{}, nil
}

// Apply applies the proxy configuration to the system.
func (p Proxy) Apply(ctx context.Context) error {
	if ctx.Value(DryRun) == true {
		log.Infof("Skipping proxy application in dry-run mode")
		return nil
	}
	log.Infof("Applying proxy configuration")

	return nil
}
