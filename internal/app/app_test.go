package app_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/ubuntu/ubuntu-proxy-manager/internal/app"
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

			_, err := app.New()
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
		applyArgs       []string
		noMethodCall    bool
		rejectAuth      bool
		proxyApplyError bool

		wantErr bool
	}{
		"Cleanly exit on correct apply arguments": {applyArgs: []string{"http://proxy:3128", "", "", "", "", ""}},
		"Timeout when no method is called on app": {noMethodCall: true},

		"Error if polkit auth is rejected":         {applyArgs: []string{"http://proxy:3128", "", "", "", "", ""}, rejectAuth: true, wantErr: true},
		"Error when applying proxy settings fails": {applyArgs: []string{"http://proxy:3128", "", "", "", "", ""}, proxyApplyError: true, wantErr: true},
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

			a, err := app.New(app.WithAuthorizer(&app.MockAuthorizer{RejectAuth: tc.rejectAuth}), app.WithProxy(&app.MockProxy{ApplyError: tc.proxyApplyError}))
			require.NoError(t, err, "Setup: New should have succeeded but didn't")

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

	_, err := app.New()
	require.NoError(t, err, "Setup: New should have succeeded but didn't")
	_, err = app.New()
	require.ErrorContains(t, err, "D-Bus name already taken")
}

func TestQuitApp(t *testing.T) {
	defer testutils.StartLocalSystemBus()()

	a, err := app.New()
	require.NoError(t, err, "Setup: New should have succeeded but didn't")
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
	case <-time.After(2 * time.Second):
		t.Fatal("App hasn't exited quickly enough")
	}
}

func TestQuitAppWithQueuedRuns(t *testing.T) {
	defer testutils.StartLocalSystemBus()()

	sleepDuration := 10 * time.Millisecond
	mockProxy := &app.MockProxy{SleepOnApply: sleepDuration}
	a, err := app.New(app.WithProxy(mockProxy), app.WithAuthorizer(&app.MockAuthorizer{}))
	require.NoError(t, err, "Setup: New should have succeeded but didn't")

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = a.Wait()
	}()

	conn := testutils.NewDbusConn(t).Object("com.ubuntu.ProxyManager", "/com/ubuntu/ProxyManager")

	// Call the D-Bus function 5 times in parallel.
	for i := 0; i < 5; i++ {
		go func() {
			err := conn.Call("com.ubuntu.ProxyManager.Apply", 0, "", "", "", "", "", "").Err
			require.NoError(t, err, "D-Bus Apply call should have succeeded but didn't")
		}()
	}

	// Sleep for 3 runs.
	time.Sleep(3 * sleepDuration)

	// Quit the app.
	a.Quit()

	// Call the D-Bus function 5 times in parallel again.
	for i := 0; i < 5; i++ {
		go func() {
			err := conn.Call("com.ubuntu.ProxyManager.Apply", 0, "", "", "", "", "", "").Err
			require.EqualError(t, err, "application is exiting")
		}()
	}

	select {
	case <-done:
		require.Equal(t, 5, mockProxy.ApplyCount, "App should have run only 5 times but didn't")
	case <-time.After(2 * time.Second):
		t.Fatal("App hasn't exited quickly enough")
	}
}

func TestMultipleRunsErrorsAreJoined(t *testing.T) {
	defer testutils.StartLocalSystemBus()()

	a, err := app.New(app.WithProxy(&app.MockProxy{ApplyError: true, SleepOnApply: 5 * time.Millisecond}), app.WithAuthorizer(&app.MockAuthorizer{}))
	require.NoError(t, err, "Setup: New should have succeeded but didn't")

	var appErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		appErr = a.Wait()
	}()

	conn := testutils.NewDbusConn(t).Object("com.ubuntu.ProxyManager", "/com/ubuntu/ProxyManager")

	// Call the D-Bus function 5 times in parallel.
	var expectedErr string
	for i := 0; i < 5; i++ {
		expectedErr += fmt.Sprintln("proxy apply error")
		go func() { _ = conn.Call("com.ubuntu.ProxyManager.Apply", 0, "", "", "", "", "", "") }()
	}

	select {
	case <-done:
		require.EqualError(t, appErr, strings.TrimSpace(expectedErr), "App should have returned multiple errors but didn't")
	case <-time.After(2 * time.Second):
		t.Fatal("App hasn't exited quickly enough")
	}
}

func TestMain(m *testing.M) {
	logrus.StandardLogger().SetLevel(logrus.DebugLevel)

	m.Run()
}
