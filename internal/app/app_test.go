package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/ubuntu/ubuntu-proxy-manager/internal/app"
	"github.com/ubuntu/ubuntu-proxy-manager/internal/proxy"
	"github.com/ubuntu/ubuntu-proxy-manager/internal/testutils"
)

func TestNew(t *testing.T) {
	tests := map[string]struct {
		noSystemBus bool

		wantErr bool
	}{
		"Create object when bus is available": {},

		"Error when system bus is not available": {noSystemBus: true, wantErr: true},
	}

	for name, tc := range tests {
		tc := tc
		name := name
		t.Run(name, func(t *testing.T) {
			if !tc.noSystemBus {
				defer testutils.StartLocalSystemBus()()
			}

			_, err := app.New(context.Background())
			if tc.wantErr {
				require.Error(t, err, "New should have failed but didn't")
				return
			}
			require.NoError(t, err, "New should have succeeded but didn't")
		})
	}
}

func TestWait(t *testing.T) {
	tests := map[string]struct {
		applyArgs    []string
		noMethodCall bool
		rejectAuth   bool

		wantErr bool
	}{
		"Cleanly exit on correct apply arguments": {applyArgs: []string{"http://proxy:3128", "", "", "", "", ""}},
		"Timeout when no method is called on app": {noMethodCall: true},

		"Error if polkit auth is rejected":        {applyArgs: []string{"http://proxy:3128", "", "", "", "", ""}, rejectAuth: true, wantErr: true},
		"Error if proxy arguments are unparsable": {applyArgs: []string{"http://pro\x7Fy:3128", "", "", "", "", ""}, wantErr: true},
	}

	for name, tc := range tests {
		tc := tc
		name := name
		t.Run(name, func(t *testing.T) {
			defer testutils.StartLocalSystemBus()()

			args := make([]interface{}, len(tc.applyArgs))
			for i := range tc.applyArgs {
				args[i] = tc.applyArgs[i]
			}

			ctx := context.WithValue(context.Background(), proxy.DryRun, true)
			a, err := app.New(ctx, app.WithAuthorizer(&app.MockAuthorizer{RejectAuth: tc.rejectAuth}))
			require.NoError(t, err, "New should have succeeded but didn't")

			done := make(chan struct{})
			go func() {
				defer close(done)
				err = a.Wait()
			}()

			conn := testutils.NewDbusConn(t).Object("com.ubuntu.ProxyManager", "/com/ubuntu/ProxyManager")

			if !tc.noMethodCall {
				dbusErr := conn.Call("com.ubuntu.ProxyManager.Apply", 0, args...).Err
				if tc.wantErr {
					require.Error(t, dbusErr, "D-Bus Apply call should have failed but didn't")
				} else {
					require.NoError(t, dbusErr, "D-Bus Apply call should have succeeded but didn't")
				}
			}

			select {
			case <-done:
				if tc.wantErr {
					require.Error(t, err, "App should have failed but didn't")
					return
				}
				require.NoError(t, err, "App should have succeeded but didn't")
			case <-time.After(5 * time.Second):
				t.Fatal("App hasn't exited quickly enough")
			}
		})
	}
}

func TestAppAlreadyExported(t *testing.T) {
	defer testutils.StartLocalSystemBus()()

	_, err := app.New(context.Background())
	require.NoError(t, err, "New should have succeeded but didn't")
	_, err = app.New(context.Background())
	require.ErrorContains(t, err, "D-Bus name already taken")
}

func TestQuitApp(t *testing.T) {
	defer testutils.StartLocalSystemBus()()

	ctx := context.WithValue(context.Background(), proxy.DryRun, true)
	a, err := app.New(ctx)
	require.NoError(t, err, "New should have succeeded but didn't")
	var appErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		appErr = a.Wait()
	}()

	a.Quit()
	select {
	case <-done:
		require.NoError(t, appErr, "App shouldn't have failed but did")
	case <-time.After(1 * time.Second):
		t.Fatal("App hasn't exited quickly enough")
	}
}

func TestMain(m *testing.M) {
	logrus.StandardLogger().SetLevel(logrus.DebugLevel)

	m.Run()
}
