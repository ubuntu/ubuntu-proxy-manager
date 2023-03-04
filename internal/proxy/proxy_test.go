package proxy_test

import (
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

// fileIsDirMsg is a placeholder message for when we create files instead of directories to trigger specific errors.
var fileIsDirMsg = "this should have been a directory\n"

func TestApply(t *testing.T) {
	t.Parallel()

	envConfigPath := proxy.DefaultEnvConfigPath
	aptConfigPath := proxy.DefaultAPTConfigPath

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
		compareTrees  bool

		wantUnchangedFiles []string
		wantErr            bool
	}{
		"No options set, no configuration files are created":       {},
		"No options set, previous configuration files are deleted": {prevContents: map[string]string{envConfigPath: "HTTP_PROXY=http://example.com:8080", aptConfigPath: `Acquire::http::Proxy "http://example.com:8080";`}},
		"HTTP option set":  {http: "http://example.com:8080"},
		"Some options set": {http: "http://example.com:8080", https: "https://example.com:8080"},
		"Some options set, configuration parent directories are created": {
			http: "http://example.com:8080", https: "https://example.com:8080", existingDirs: []string{},
		},
		"Some options set, configuration files should not be changed": {
			http: "http://example.com:8080", https: "https://example.com:8080",
			prevContents: map[string]string{
				envConfigPath: fmt.Sprintf(`%s
HTTP_PROXY=http://example.com:8080
http_proxy=http://example.com:8080
HTTPS_PROXY=https://example.com:8080
https_proxy=https://example.com:8080
`, proxy.ConfHeader),
				aptConfigPath: fmt.Sprintf(`%s
Acquire::http::Proxy "http://example.com:8080";
Acquire::https::Proxy "https://example.com:8080";
`, proxy.ConfHeader)},
			wantUnchangedFiles: []string{envConfigPath, aptConfigPath},
		},
		"All options set": {http: "http://example.com:8080", https: "https://example.com:8080", ftp: "ftp://example.com:8080", socks: "socks://example.com:8080", noProxy: "localhost,127.0.0.1"},
		"All options set and equal, all_proxy is set": {http: "http://example.com:8080", https: "http://example.com:8080", ftp: "http://example.com:8080", socks: "http://example.com:8080"},

		// Authentication / escape use cases
		"Password is escaped":                          {http: "http://username:p@$$:w0rd@example.com:8080"},
		"Escaped password is not escaped again":        {http: "http://username:p%40$$%3Aw0rd@example.com:8080"},
		"Domain username is escaped":                   {http: `http://EXAMPLE\bobsmith:p@$$:w0rd@example.com:8080`},
		"Domain username without password is escaped":  {http: `http://EXAMPLE\bobsmith@example.com:8080`},
		"Escaped domain username is not escaped again": {http: `http://EXAMPLE%5Cbobsmith@example.com:8080`},
		"Options are applied on read-only conf files":  {http: "http://example.com:8080", existingPerms: map[string]os.FileMode{envConfigPath: 0444, aptConfigPath: 0444}, prevContents: map[string]string{envConfigPath: "something", aptConfigPath: "something"}},

		// Special cases - not all files are changed
		"HTTP option set, APT file is already up to date": {
			http:               "http://example.com:8080",
			prevContents:       map[string]string{aptConfigPath: fmt.Sprintf("%s\nAcquire::http::Proxy \"http://example.com:8080\";\n", proxy.ConfHeader)},
			wantUnchangedFiles: []string{aptConfigPath},
		},
		"HTTP option set, environment file is already up to date": {
			http: "http://example.com:8080",
			prevContents: map[string]string{envConfigPath: fmt.Sprintf(`%s
HTTP_PROXY=http://example.com:8080
http_proxy=http://example.com:8080
`, proxy.ConfHeader)},
			wantUnchangedFiles: []string{envConfigPath},
		},

		// Error cases - apply
		"Error when we cannot write to the environment directory": {http: "http://example.com:8080", existingDirs: []string{"etc/"}, prevContents: map[string]string{filepath.Dir(envConfigPath): fileIsDirMsg}, compareTrees: true, wantErr: true},
		"Error when we cannot write to the APT config directory":  {http: "http://example.com:8080", existingDirs: []string{"etc/apt"}, prevContents: map[string]string{filepath.Dir(aptConfigPath): fileIsDirMsg}, compareTrees: true, wantErr: true},
		"Error when all directories are unwritable":               {http: "http://example.com:8080", existingDirs: []string{"etc/apt"}, prevContents: map[string]string{filepath.Dir(envConfigPath): fileIsDirMsg, filepath.Dir(aptConfigPath): fileIsDirMsg}, compareTrees: true, wantErr: true},
		"Error when environment directory is not readable":        {existingPerms: map[string]os.FileMode{filepath.Dir(envConfigPath): 0444}, wantErr: true},
		"Error when APT config directory is not readable":         {existingPerms: map[string]os.FileMode{filepath.Dir(aptConfigPath): 0444}, wantErr: true},

		// Error cases - setting parsing
		"Error on unparsable URI for HTTP":  {http: "http://pro\x7Fy:3128", wantErr: true},
		"Error on unparsable URI for HTTPS": {https: "http://pro\x7Fy:3128", wantErr: true},
		"Error on unparsable URI for FTP":   {ftp: "http://pro\x7Fy:3128", wantErr: true},
		"Error on unparsable URI for SOCKS": {socks: "http://pro\x7Fy:3128", wantErr: true},
		"Error on missing scheme":           {socks: "example.com:8080", wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tc.existingDirs == nil {
				tc.existingDirs = []string{filepath.Dir(envConfigPath), filepath.Dir(aptConfigPath)}
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

			p := proxy.New(proxy.WithRoot(root))
			err := p.Apply(tc.http, tc.https, tc.ftp, tc.socks, tc.noProxy, tc.mode)

			if tc.wantErr {
				require.Error(t, err, "Apply should have failed but didn't")
				// For some tests it adds value to compare directory trees (e.g. partial errors)
				if !tc.compareTrees {
					return
				}
			} else {
				require.NoError(t, err, "Apply failed but shouldn't have")
			}

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
