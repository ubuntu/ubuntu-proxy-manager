// Package app exports a D-Bus object to apply proxy settings.
package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	log "github.com/sirupsen/logrus"
	"github.com/ubuntu/decorate"
	"github.com/ubuntu/ubuntu-proxy-manager/internal/authorizer"
	"github.com/ubuntu/ubuntu-proxy-manager/internal/lifecycle"
	"github.com/ubuntu/ubuntu-proxy-manager/internal/proxy"
)

var (
	// Version is the version of the program.
	Version = "dev"
)

const (
	dbusObjectPath = "/com/ubuntu/ProxyManager"
	dbusInterface  = "com.ubuntu.ProxyManager"

	polkitApplyAction = "com.ubuntu.ProxyManager.apply"
)

const timeout = 1 * time.Second

// proxyManagerBus is the object exported to the D-Bus interface.
type proxyManagerBus struct {
	l          *lifecycle.Lifecycle
	authorizer authorizerer
	proxy      proxyApplier
}

// App is the main application object.
type App struct {
	busObject *proxyManagerBus
}

type options struct {
	authorizer authorizerer
	proxy      proxyApplier
}
type option func(*options)

type authorizerer interface {
	IsSenderAllowed(string, dbus.Sender) error
}
type proxyApplier interface {
	Apply(string, string, string, string, string, string) error
}

// Apply is a function called via D-Bus to apply the system proxy settings.
func (b *proxyManagerBus) Apply(sender dbus.Sender, http, https, ftp, socks, no, mode string) *dbus.Error {
	log.Debugf("Sender %s called Apply(%q, %q, %q, %q, %q, %q)", sender, http, https, ftp, socks, no, mode)

	// Application was already asked to quit, so return an error without applying anything
	if b.l.QuitRequested() {
		return dbus.MakeFailedError(errors.New("application exit requested, cannot apply proxy settings"))
	}

	// Methods calls spin up separate goroutines, so ensure we don't run them in parallel
	b.l.Start()

	// Signal to the lifecycle that we finished the run and pass any error to it
	var err error
	defer func() { b.l.RunDone(err) }()

	// Check if the caller is authorized to call this method
	if err = b.authorizer.IsSenderAllowed(polkitApplyAction, sender); err != nil {
		return dbus.MakeFailedError(err)
	}

	if err = b.proxy.Apply(http, https, ftp, socks, no, mode); err != nil {
		return dbus.MakeFailedError(err)
	}
	return nil
}

// New creates a new App object.
func New(ctx context.Context, args ...option) (a *App, err error) {
	defer decorate.OnError(&err, "cannot initialize application")

	// Don't call dbus.SystemBus which caches globally system dbus (issues in tests)
	// Add interceptor to log dbus messages at debug level
	// Pass context to dbus connection so we handle closing it on context cancel
	conn, err := dbus.ConnectSystemBus(
		dbus.WithIncomingInterceptor(func(msg *dbus.Message) {
			log.Debugf("DBUS: %s", msg)
		}))
	if err != nil {
		return nil, err
	}

	// Set default options
	opts := options{
		authorizer: authorizer.New(conn),
		proxy:      proxy.New(),
	}

	// Apply given options
	for _, f := range args {
		f(&opts)
	}

	lifecycle := lifecycle.New(timeout)
	obj := proxyManagerBus{
		l:          lifecycle,
		authorizer: opts.authorizer,
		proxy:      opts.proxy,
	}

	if err = conn.Export(&obj, dbusObjectPath, dbusInterface); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err = conn.Export(introspect.NewIntrospectable(&introspect.Node{
		Name: dbusObjectPath,
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			{
				Name:    dbusInterface,
				Methods: introspect.Methods(&obj),
			},
		},
	}), dbusObjectPath, introspect.IntrospectData.Name); err != nil {
		_ = conn.Close()
		return nil, err
	}

	reply, err := conn.RequestName(dbusInterface, dbus.NameFlagDoNotQueue)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		_ = conn.Close()
		return nil, fmt.Errorf("D-Bus name already taken")
	}

	return &App{
		busObject: &obj,
	}, nil
}

// Wait blocks until the context is cancelled or the timeout is reached,
// returning an error if applicable.
// As this is a one-shot program, we let the system handle cancelling the dbus connection.
func (a *App) Wait() error {
	if err := a.busObject.l.Wait(); errors.Is(err, lifecycle.ErrTimeoutReached) {
		log.Debug("Timeout exceeded, exiting...")
		return nil
	} else if err != nil {
		return err
	}

	return nil
}

// Quit stops the application, waiting for it to finish if we're in the process
// of applying the proxy configuration.
func (a *App) Quit() {
	log.Info("Exiting program on user request...")
	a.busObject.l.Quit()
}
