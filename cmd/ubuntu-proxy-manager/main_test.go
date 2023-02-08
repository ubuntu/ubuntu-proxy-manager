package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/ubuntu/ubuntu-proxy-manager/internal/app"
)

type myApp struct {
	done chan struct{}

	waitError bool
}

func (a *myApp) Wait() error {
	<-a.done
	if a.waitError {
		return errors.New("Error requested for Wait")
	}
	return nil
}

func (a *myApp) Quit() {
	close(a.done)
}

func TestRun(t *testing.T) {
	tests := map[string]struct {
		args []string

		waitError bool
		sendSig   syscall.Signal

		wantOut      string
		wantErr      string
		wantLogLevel logrus.Level

		wantReturnCode int
	}{
		"Run and exit successfully": {},
		"Accept short help flag":    {args: []string{"-h"}, wantErr: "ubuntu-proxy-manager [options]"},
		"Accept long help flag":     {args: []string{"--help"}, wantErr: "ubuntu-proxy-manager [options]"},
		"Accept short version flag": {args: []string{"-v"}, wantOut: app.Version},
		"Accept long version flag":  {args: []string{"--version"}, wantOut: app.Version},
		"Accept short debug flag":   {args: []string{"-d"}, wantLogLevel: logrus.DebugLevel},
		"Accept long debug flag":    {args: []string{"--debug"}, wantLogLevel: logrus.DebugLevel},

		"Error if wait fails":                 {waitError: true, wantReturnCode: 1},
		"Error when passed any argument":      {args: []string{"bad-arg"}, wantReturnCode: 2},
		"Error when passed bad options":       {args: []string{"-bad-opt"}, wantReturnCode: 2},
		"Error when passed bad POSIX options": {args: []string{"--bad-opt"}, wantReturnCode: 2},

		// Signals handling
		"Send SIGINT exits":  {sendSig: syscall.SIGINT},
		"Send SIGTERM exits": {sendSig: syscall.SIGTERM},
		"Send SIGHUP exits":  {sendSig: syscall.SIGHUP},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			args := []string{"ubuntu-proxy-manager"}
			initOsArgs := os.Args
			defer func() { os.Args = initOsArgs }()
			os.Args = append(args, tc.args...)

			a := myApp{
				done:      make(chan struct{}),
				waitError: tc.waitError,
			}

			initOut, initErr := os.Stdout, os.Stderr
			defer func() { os.Stdout, os.Stderr = initOut, initErr }()
			rOut, wOut, err := os.Pipe()
			require.NoError(t, err, "Setup: couldn't create pipe for stdout")
			rErr, wErr, err := os.Pipe()
			require.NoError(t, err, "Setup: couldn't create pipe for stderr")
			os.Stdout, os.Stderr = wOut, wErr

			var rc int
			wait := make(chan struct{})
			go func() {
				rc = run(&a)
				close(wait)
			}()

			time.Sleep(50 * time.Millisecond)

			err = wOut.Close()
			require.NoError(t, err, "Setup: couldn't close pipe for stdout")
			os.Stdout = initOut
			err = wErr.Close()
			require.NoError(t, err, "Setup: couldn't close pipe for stderr")
			os.Stderr = initErr

			var bufOut, bufErr bytes.Buffer
			_, err = io.Copy(&bufOut, rOut)
			require.NoError(t, err, "Setup: couldn't read stdout")
			_, err = io.Copy(&bufErr, rErr)
			require.NoError(t, err, "Setup: couldn't read stderr")

			if tc.wantOut != "" {
				require.Contains(t, bufOut.String(), tc.wantOut, "stdout doesn't contain expected output")
			}
			if tc.wantErr != "" {
				require.Contains(t, bufErr.String(), tc.wantErr, "stderr doesn't contain expected output")
			}

			var exited bool
			if tc.sendSig != 0 {
				err := syscall.Kill(syscall.Getpid(), tc.sendSig)
				require.NoError(t, err, "Teardown: kill should return no error")
				select {
				case <-time.After(50 * time.Millisecond):
					exited = false
				case <-wait:
					exited = true
				}
				require.Equal(t, true, exited, "Expect to exit on SIGINT, SIGTERM or SIGHUP")
			}

			if !exited {
				a.Quit()
				<-wait
			}

			require.Equal(t, tc.wantReturnCode, rc, "Return expected code")
		})
	}
}
