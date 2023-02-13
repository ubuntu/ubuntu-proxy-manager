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
	"github.com/ubuntu/ubuntu-proxy-manager/internal/proxy"
)

var (
	// Version is the version of the program.
	Version = "dev"
)

const (
	dbusObjectPath = "/com/ubuntu/ProxyManager"
	dbusInterface  = "com.ubuntu.ProxyManager"
)

const timeout = 1 * time.Second

// proxyManagerBus is the object exported to the D-Bus interface.
type proxyManagerBus struct {
	ctx     context.Context
	cancel  context.CancelCauseFunc
	running bool

	mu sync.Mutex
}

// App is the main application object.
type App struct {
	busObject *proxyManagerBus
}

// Apply is a function called via D-Bus to apply the system proxy settings.
func (b *proxyManagerBus) Apply(http, https, ftp, socks, no, mode string) *dbus.Error {
	// Methods calls spin up separate goroutines, so ensure we don't run them in parallel
	b.mu.Lock()
	defer b.mu.Unlock()

	b.running = true

	log.Infof("Called Apply(%s, %s, %s, %s, %s, %s)", http, https, ftp, socks, no, mode)

	p, err := proxy.New(b.ctx, http, https, ftp, socks, no, mode)
	// We need to cancel the context in a deferred function to get the final
	// state of the error variable, and to let the main thread know it's safe to quit.
	defer func() { b.running = false; b.cancel(err) }()

	if err != nil {
		return dbus.MakeFailedError(err)
	}

	err = p.Apply(b.ctx)
	if err != nil {
		return dbus.MakeFailedError(err)
	}
	return nil
}

// New creates a new App object.
func New(ctx context.Context) (a *App, err error) {
	defer decorate.OnError(&err, "cannot initialize application")

	ctx, cancel := context.WithCancelCause(ctx)
	obj := proxyManagerBus{
		ctx:    ctx,
		cancel: cancel,
	}

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
	for {
		select {
		case <-time.After(timeout):
			// Wait for any running apply to finish
			if a.busObject.running {
				<-a.busObject.ctx.Done()
			}
			return nil
		case <-a.busObject.ctx.Done():
			if err := context.Cause(a.busObject.ctx); err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
			return nil // success
		}
	}
}

// Quit stops the application, waiting for it to finish if we're in the process
// of applying the proxy configuration.
func (a *App) Quit() {
	log.Info("Exiting program on user request...")
	// Wait for the Apply method to finish if applicable
	if a.busObject.running {
		log.Warning("An Apply call is running, waiting for it to finish")
		<-a.busObject.ctx.Done()
		return
	}

	// Otherwise just cancel the context
	a.busObject.cancel(nil)
}
