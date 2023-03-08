// Package app exports a D-Bus object to apply proxy settings.
package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	log "github.com/sirupsen/logrus"
	"github.com/ubuntu/decorate"
	"github.com/ubuntu/ubuntu-proxy-manager/internal/authorizer"
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
	authorizer authorizerer
	proxy      proxyApplier

	applyCalls    chan applyCall
	applyResponse chan error

	exited bool
	exitMu sync.RWMutex
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
	CheckSenderAllowed(string, dbus.Sender) error
}
type proxyApplier interface {
	Apply(string, string, string, string, string, string) error
}

type applyCall struct {
	sender dbus.Sender

	http  string
	https string
	ftp   string
	socks string
	no    string
	auto  string
}

// Apply is a function called via D-Bus to apply the system proxy settings.
func (b *proxyManagerBus) Apply(sender dbus.Sender, http, https, ftp, socks, no, auto string) *dbus.Error {
	// Application was already asked to quit, so return an error without applying anything
	if b.QuitRequested() {
		return dbus.MakeFailedError(errors.New("application is exiting"))
	}

	// Send the request to the main loop
	b.applyCalls <- applyCall{sender, http, https, ftp, socks, no, auto}

	// Wait for the main loop to process the request
	if err := <-b.applyResponse; err != nil {
		return dbus.MakeFailedError(err)
	}
	return nil
}

func (b *proxyManagerBus) apply(args applyCall) error {
	log.Debugf("Sender %s called Apply: %v", args.sender, args)

	if err := b.authorizer.CheckSenderAllowed(polkitApplyAction, args.sender); err != nil {
		return err
	}
	return b.proxy.Apply(args.http, args.https, args.ftp, args.socks, args.no, args.auto)
}

// QuitRequested returns true if the application has been requested to quit.
func (b *proxyManagerBus) QuitRequested() bool {
	b.exitMu.RLock()
	defer b.exitMu.RUnlock()

	return b.exited
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

	obj := proxyManagerBus{
		authorizer:    opts.authorizer,
		proxy:         opts.proxy,
		applyCalls:    make(chan applyCall),
		applyResponse: make(chan error),
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

// Wait blocks until the all operations are done, returning a joined
// representation of all errors that occurred during the runs.
func (a *App) Wait() error {
	var globalErr error
	for {
		select {
		case call := <-a.busObject.applyCalls:
			err := a.busObject.apply(call)
			globalErr = errors.Join(globalErr, err)
			a.busObject.applyResponse <- err
		case <-time.After(timeout):
			return globalErr
		}
	}
}

// Quit signals the application to stop, waiting for current operations to finish.
func (a *App) Quit() {
	log.Info("Exiting program on user request...")

	a.busObject.exitMu.Lock()
	defer a.busObject.exitMu.Unlock()

	a.busObject.exited = true
}
