package authorizer_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/ubuntu-proxy-manager/internal/authorizer"
	"github.com/ubuntu/ubuntu-proxy-manager/internal/testutils"
)

func TestCheckSenderAllowed(t *testing.T) {
	t.Cleanup(testutils.StartLocalSystemBus())

	t.Parallel()

	bus := testutils.NewDbusConn(t)

	tests := map[string]struct {
		pid uint32
		uid uint32

		credsUID        any
		credsPID        any
		polkitAuthorize bool

		wantPolkitError      bool
		wantCredsLookupError bool

		wantErr bool
	}{
		"Root is always authorized":        {uid: 0},
		"Valid process and UID authorized": {pid: 10000, uid: 1000, polkitAuthorize: true},

		// Unauthorized cases
		"Unauthorized if polkit call returns an error":     {pid: 10000, uid: 1000, wantPolkitError: true, wantErr: true},
		"Unauthorized if polkit did not authorize":         {pid: 10000, uid: 1000, polkitAuthorize: false, wantErr: true},
		"Unauthorized if creds lookup returns an error":    {pid: 10000, uid: 1000, wantCredsLookupError: true, polkitAuthorize: true, wantErr: true},
		"Unauthorized if creds lookup UID is not a number": {pid: 10000, uid: 1000, credsUID: "NaN", polkitAuthorize: true, wantErr: true},
		"Unauthorized if creds lookup PID is not a number": {pid: 10000, uid: 1000, credsPID: "NaN", polkitAuthorize: true, wantErr: true},

		// Unauthorized - bad PID files
		"Unauthorized if PID file does not exist on the system":          {pid: 99999, uid: 1000, polkitAuthorize: true, wantErr: true},
		"Unauthorized on invalid process stat file: missing )":           {pid: 10001, uid: 1000, polkitAuthorize: true, wantErr: true},
		"Unauthorized on invalid process stat file: ) at the end":        {pid: 10002, uid: 1000, polkitAuthorize: true, wantErr: true},
		"Unauthorized on invalid process stat file: field isn't present": {pid: 10003, uid: 1000, polkitAuthorize: true, wantErr: true},
		"Unauthorized on invalid process stat file: field isn't an int":  {pid: 10004, uid: 1000, polkitAuthorize: true, wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Set reasonable defaults if returned credentials are not overridden
			if tc.credsUID == nil {
				tc.credsUID = tc.uid
			}
			if tc.credsPID == nil {
				tc.credsPID = tc.pid
			}

			a := authorizer.New(
				bus,
				authorizer.WithAuthority(&authorizer.PolkitObjMock{IsAuthorized: tc.polkitAuthorize, WantPolkitError: tc.wantPolkitError}),
				authorizer.WithCredLookup(&authorizer.CredsObjMock{UID: tc.credsUID, PID: tc.credsPID, WantLookupError: tc.wantCredsLookupError}),
				authorizer.WithRoot("testdata"),
			)

			if tc.wantErr {
				require.Error(t, a.CheckSenderAllowed("my-action", "sender"))
				return
			}
			require.NoError(t, a.CheckSenderAllowed("my-action", "sender"))
		})
	}
}
