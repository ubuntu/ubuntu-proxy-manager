package proxy_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/ubuntu-proxy-manager/internal/proxy"
	"github.com/ubuntu/ubuntu-proxy-manager/internal/testutils"
)

func TestApply(t *testing.T) {
	t.Parallel()

	envConfigPath := proxy.DefaultEnvConfigPath
	initialTime := time.Unix(0, 0).UTC()

	tests := map[string]struct {
		http    string
		https   string
		ftp     string
		socks   string
		noProxy string
		mode    string

		existingDirs  []string
		existingPerms map[string]os.FileMode
		prevContents  map[string]string
		dryRun        bool

		wantUnchangedFiles []string
		wantErr            bool
	}{
		"No options set, no environment file is created":       {},
		"No options set, previous environment file is deleted": {prevContents: map[string]string{envConfigPath: "HTTP_PROXY=http://example.com:8080"}},
		"HTTP option set":  {http: "http://example.com:8080"},
		"Some options set": {http: "http://example.com:8080", https: "https://example.com:8080"},
		"Some options set, environment parent directories are created": {
			http: "http://example.com:8080", https: "https://example.com:8080", existingDirs: []string{},
		},
		"Some options set, environment file should not be changed": {
			http: "http://example.com:8080", https: "https://example.com:8080",
			prevContents: map[string]string{envConfigPath: fmt.Sprintf(`%s
HTTP_PROXY=http://example.com:8080
http_proxy=http://example.com:8080
HTTPS_PROXY=https://example.com:8080
https_proxy=https://example.com:8080
`, proxy.ConfHeader)},
			wantUnchangedFiles: []string{envConfigPath},
		},
		"All options set": {http: "http://example.com:8080", https: "https://example.com:8080", ftp: "ftp://example.com:8080", socks: "socks://example.com:8080", noProxy: "localhost,127.0.0.1"},
		"All options set and equal, all_proxy is set": {http: "http://example.com:8080", https: "http://example.com:8080", ftp: "http://example.com:8080", socks: "http://example.com:8080"},

		// Authentication / escape use cases
		"Password is escaped":                          {http: "http://username:p@$$:w0rd@example.com:8080"},
		"Escaped password is not escaped again":        {http: "http://username:p%40$$%3Aw0rd@example.com:8080"},
		"Domain username is escaped":                   {http: `http://EXAMPLE\bobsmith:p@$$:w0rd@example.com:8080`},
		"Domain username without password is escaped":  {http: `http://EXAMPLE\bobsmith@example.com:8080`},
		"Escaped domain username is not escaped again": {http: `http://EXAMPLE%5Cbobsmith@example.com:8080`},
		"Options are applied on read-only env file":    {http: "http://example.com:8080", existingPerms: map[string]os.FileMode{envConfigPath: 0444}, prevContents: map[string]string{envConfigPath: "something"}},

		"Proxy files not created when ran with dry-run": {dryRun: true},

		// Error cases - apply
		"Error when we can't write to the environment directory": {existingDirs: []string{"etc/"}, prevContents: map[string]string{filepath.Dir(envConfigPath): "something"}, wantErr: true},
		"Error when environment directory is not readable":       {existingPerms: map[string]os.FileMode{filepath.Dir(envConfigPath): 0444}, wantErr: true},

		// Error cases - setting parsing
		"Error on unparsable URI for HTTP":          {http: "http://pro\x7Fy:3128", https: "https://valid.proxy", wantErr: true},
		"Error on unparsable URI for HTTP (global)": {http: "http://pro\x7Fy:3128", wantErr: true},
		"Error on unparsable URI for HTTPS":         {https: "http://pro\x7Fy:3128", wantErr: true},
		"Error on unparsable URI for FTP":           {ftp: "http://pro\x7Fy:3128", wantErr: true},
		"Error on unparsable URI for SOCKS":         {socks: "http://pro\x7Fy:3128", wantErr: true},
		"Error on missing scheme":                   {socks: "example.com:8080", wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tc.existingDirs == nil {
				tc.existingDirs = []string{filepath.Dir(envConfigPath)}
			}

			root := t.TempDir()
			for _, p := range tc.existingDirs {
				err := os.MkdirAll(filepath.Join(root, p), 0700)
				require.NoError(t, err, "Setup: Couldn't create %s", p)
			}

			for p, c := range tc.prevContents {
				err := os.WriteFile(filepath.Join(root, p), []byte(c), 0600)
				require.NoError(t, err, "Setup: Couldn't write previous contents to %q", p)

				err = os.Chtimes(filepath.Join(root, p), time.Now().UTC(), initialTime)
				require.NoError(t, err, "Setup: Couldn't change mtime for %q", p)
			}

			for file, perms := range tc.existingPerms {
				testutils.Chmod(t, filepath.Join(root, file), perms)
			}

			ctx := context.Background()
			if tc.dryRun {
				ctx = context.WithValue(ctx, proxy.DryRun, true)
			}

			p := proxy.New(ctx, proxy.WithRoot(root))
			err := p.Apply(ctx, tc.http, tc.https, tc.ftp, tc.socks, tc.noProxy, tc.mode)
			if tc.wantErr {
				require.Error(t, err, "Apply should have failed but didn't")
				return
			}
			require.NoError(t, err, "Apply failed but shouldn't have")

			testutils.CompareTreesWithFiltering(t, root, testutils.GoldenPath(t), testutils.Update())
			for _, file := range tc.wantUnchangedFiles {
				fi, err := os.Stat(filepath.Join(root, file))
				require.NoError(t, err, "Setup: Failed to stat proxy config file")
				require.Equal(t, initialTime, fi.ModTime().UTC(), "Proxy config file mtime should not have changed")
			}
		})
	}
}

func TestMain(m *testing.M) {
	testutils.InstallUpdateFlag()
	flag.Parse()

	m.Run()
}
