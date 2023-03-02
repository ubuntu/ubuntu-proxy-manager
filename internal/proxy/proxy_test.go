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

const (
	// fileIsDirMsg is a placeholder message for when we create files instead of directories to trigger specific errors.
	fileIsDirMsg = "this should have been a directory\n"

	// glibCompileSchemasRunFile is the file that should be created when the mock glib-compile-schemas is run.
	glibCompileSchemasRunFile = ".ran-glib-compile-schemas"
)

func TestApply(t *testing.T) {
	t.Parallel()

	envConfigPath := proxy.DefaultEnvConfigPath
	aptConfigPath := proxy.DefaultAPTConfigPath
	gsettingsConfigPath := proxy.DefaultGSettingsConfigPath

	initialTime := time.Unix(0, 0).UTC()

	tests := map[string]struct {
		http    string
		https   string
		ftp     string
		socks   string
		noProxy string
		auto    string

		existingDirs  []string
		existingPerms map[string]os.FileMode
		prevContents  map[string]string
		compareTrees  bool

		glibMockError         bool
		missingGlibExecutable bool

		wantUnchangedFiles []string
		wantGlibMockNotRun bool
		wantErr            bool
	}{
		"No options set, no configuration files are created": {wantGlibMockNotRun: true},
		"No options set, previous configuration files are deleted": {
			prevContents: map[string]string{
				envConfigPath:       "HTTP_PROXY=http://example.com:8080",
				aptConfigPath:       `Acquire::http::Proxy "http://example.com:8080";`,
				gsettingsConfigPath: "[org.gnome.system.proxy.http]\nhost='example.com'\nport=8080\n",
			}},
		"HTTP option set":  {http: "http://example.com:8080"},
		"Some options set": {http: "http://example.com:8080", https: "https://example.com:8080"},
		"Some options set, configuration parent directories are created for env and APT": {
			http: "http://example.com:8080", https: "https://example.com:8080", existingDirs: []string{proxy.DefaultGLibSchemaPath},
		},
		"Some options set, configuration files should not be changed": {
			http: "http://example.com:8080", https: "https://example.com:8080",
			prevContents: map[string]string{
				envConfigPath: fmt.Sprintf(`%s
HTTP_PROXY="http://example.com:8080"
http_proxy="http://example.com:8080"
HTTPS_PROXY="https://example.com:8080"
https_proxy="https://example.com:8080"
`, proxy.ConfHeader),
				aptConfigPath: fmt.Sprintf(`%s
Acquire::http::Proxy "http://example.com:8080";
Acquire::https::Proxy "https://example.com:8080";
`, proxy.ConfHeader),
				gsettingsConfigPath: fmt.Sprintf(`%s
[org.gnome.system.proxy.http]
host='example.com'
port=8080

[org.gnome.system.proxy.https]
host='example.com'
port=8080

[org.gnome.system.proxy]
mode='manual'
`, proxy.ConfHeader),
			},
			wantGlibMockNotRun: true,
			wantUnchangedFiles: []string{envConfigPath, aptConfigPath, gsettingsConfigPath},
		},
		"All options set": {http: "http://example.com:8080", https: "https://example.com:8080", ftp: "ftp://example.com:8080", socks: "socks://example.com:8080", noProxy: "localhost,127.0.0.1", auto: "http://example.com:8080/proxy.pac"},
		"All options set and equal, all_proxy is set": {http: "http://example.com:8080", https: "http://example.com:8080", ftp: "http://example.com:8080", socks: "http://example.com:8080"},

		// Authentication / escape use cases
		// not applicable to GSettings
		"Password is escaped":                          {http: "http://username:p@$$:w0rd@example.com:8080"},
		"Escaped password is not escaped again":        {http: "http://username:p%40$$%3Aw0rd@example.com:8080"},
		"Domain username is escaped":                   {http: `http://EXAMPLE\bobsmith:p@$$:w0rd@example.com:8080`},
		"Domain username without password is escaped":  {http: `http://EXAMPLE\bobsmith@example.com:8080`},
		"Escaped domain username is not escaped again": {http: `http://EXAMPLE%5Cbobsmith@example.com:8080`},
		// applicable to GSettings
		"Ignored hosts are wrapped in single quotes for GSettings":               {noProxy: "localhost,127.0.0.1,::1"},
		"Double quoted ignored hosts are changed to single quotes for GSettings": {noProxy: `"localhost","127.0.0.1","::1"`},
		"Single quoted ignored hosts are not touched for GSettings":              {noProxy: "'localhost','127.0.0.1','::1'"},

		// Special cases
		"Options are applied on read-only conf files": {http: "http://example.com:8080",
			existingPerms: map[string]os.FileMode{envConfigPath: 0444, aptConfigPath: 0444, gsettingsConfigPath: 0444},
			prevContents:  map[string]string{envConfigPath: "something", aptConfigPath: "something", gsettingsConfigPath: "something"}},
		"HTTP option set, APT file is already up to date": {
			http:               "http://example.com:8080",
			prevContents:       map[string]string{aptConfigPath: fmt.Sprintf("%s\nAcquire::http::Proxy \"http://example.com:8080\";\n", proxy.ConfHeader)},
			wantUnchangedFiles: []string{aptConfigPath},
		},
		"HTTP option set, environment file is already up to date": {
			http: "http://example.com:8080",
			prevContents: map[string]string{envConfigPath: fmt.Sprintf(`%s
HTTP_PROXY="http://example.com:8080"
http_proxy="http://example.com:8080"
`, proxy.ConfHeader)},
			wantUnchangedFiles: []string{envConfigPath},
		},
		"HTTP option set, GSettings file is already up to date": {
			http: "http://example.com:8080",
			prevContents: map[string]string{gsettingsConfigPath: fmt.Sprintf(`%s
[org.gnome.system.proxy.http]
host='example.com'
port=8080

[org.gnome.system.proxy]
mode='manual'
`, proxy.ConfHeader)},
			wantGlibMockNotRun: true,
			wantUnchangedFiles: []string{gsettingsConfigPath},
		},
		"HTTP and HTTPS set with authentication, GSettings file only contains HTTP auth": {
			http:  "http://username:p@$$w0rd@example.com:8080",
			https: "http://username:p@$$w0rd@example.com:8080",
		},
		"Do not error if glib-compile-schemas is not found": {http: "http://example.com:8080", missingGlibExecutable: true, wantGlibMockNotRun: true},
		"Auto proxy is skipped by environment":              {auto: "http://example.com:8080/proxy.pac"},
		"Auto proxy and no proxy are skipped by APT":        {auto: "http://example.com:8080/proxy.pac", noProxy: "localhost,127.0.0.1"},

		// Error cases - apply
		"Error when we cannot write to the environment directory": {http: "http://example.com:8080", existingDirs: []string{proxy.DefaultGLibSchemaPath, "etc/"}, prevContents: map[string]string{filepath.Dir(envConfigPath): fileIsDirMsg}, compareTrees: true, wantErr: true},
		"Error when we cannot write to the APT config directory":  {http: "http://example.com:8080", existingDirs: []string{proxy.DefaultGLibSchemaPath, "etc/apt"}, prevContents: map[string]string{filepath.Dir(aptConfigPath): fileIsDirMsg}, compareTrees: true, wantErr: true},
		"Error when we cannot write to the GLib schema directory": {http: "http://example.com:8080", existingDirs: []string{"usr/share/glib-2.0"}, prevContents: map[string]string{filepath.Dir(gsettingsConfigPath): fileIsDirMsg}, compareTrees: true, wantGlibMockNotRun: true, wantErr: true},
		"Error when some directories are unwritable": {http: "http://example.com:8080", existingDirs: []string{"etc", "usr/share/glib-2.0"},
			prevContents: map[string]string{filepath.Dir(envConfigPath): fileIsDirMsg, filepath.Dir(gsettingsConfigPath): fileIsDirMsg}, compareTrees: true, wantGlibMockNotRun: true, wantErr: true},
		"Error when all directories are unwritable": {http: "http://example.com:8080", existingDirs: []string{"etc/apt", "usr/share/glib-2.0"},
			prevContents: map[string]string{filepath.Dir(envConfigPath): fileIsDirMsg, filepath.Dir(aptConfigPath): fileIsDirMsg, filepath.Dir(gsettingsConfigPath): fileIsDirMsg},
			compareTrees: true, wantGlibMockNotRun: true, wantErr: true},

		"Error when environment directory is not readable": {existingPerms: map[string]os.FileMode{filepath.Dir(envConfigPath): 0444}, wantErr: true},
		"Error when APT config directory is not readable":  {existingPerms: map[string]os.FileMode{filepath.Dir(aptConfigPath): 0444}, wantErr: true},
		"Error when GLib schema directory is not readable": {existingPerms: map[string]os.FileMode{filepath.Dir(gsettingsConfigPath): 0444}, wantGlibMockNotRun: true, wantErr: true},
		"Error when some directories are not readable":     {existingPerms: map[string]os.FileMode{filepath.Dir(aptConfigPath): 0444, filepath.Dir(gsettingsConfigPath): 0444}, wantGlibMockNotRun: true, wantErr: true},
		"Error when all directories are not readable":      {existingPerms: map[string]os.FileMode{filepath.Dir(envConfigPath): 0444, filepath.Dir(aptConfigPath): 0444, filepath.Dir(gsettingsConfigPath): 0444}, wantGlibMockNotRun: true, wantErr: true},

		"Error when GLib schema directory does not exist": {existingDirs: []string{}, wantGlibMockNotRun: true, wantErr: true},
		"Error when glib-compile-schemas fails":           {http: "http://example.com:8080", glibMockError: true, wantGlibMockNotRun: true, wantErr: true},
		"Error when glib-compile-schemas fails, previous config file is restored": {
			http: "http://example.com:8080", prevContents: map[string]string{gsettingsConfigPath: "some-old-contents\n"},
			glibMockError: true, compareTrees: true, wantGlibMockNotRun: true, wantErr: true},

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
				tc.existingDirs = []string{filepath.Dir(envConfigPath), filepath.Dir(aptConfigPath), filepath.Dir(gsettingsConfigPath)}
			}

			// A root directory to use for golden tree comparison, and a temp
			// scratch directory to use for the test.
			root, temp := t.TempDir(), t.TempDir()
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

			// Handle mocking glib-compile-schemas
			mockGlibCmd := mockGlibCompileSchemasCmd(t, temp)
			if tc.glibMockError {
				mockGlibCmd = append(mockGlibCmd, "-Exit1-")
			} else {
				mockGlibCmd = append(mockGlibCmd, "-Exit0-")
			}

			if tc.missingGlibExecutable {
				mockGlibCmd = []string{"not-an-executable-hopefully"}
			}

			ctx := context.Background()
			p := proxy.New(ctx, proxy.WithRoot(root), proxy.WithGlibCompileSchemasCmd(mockGlibCmd))
			err := p.Apply(ctx, tc.http, tc.https, tc.ftp, tc.socks, tc.noProxy, tc.auto)

			if tc.wantErr {
				require.Error(t, err, "Apply should have failed but didn't")
				// For some tests it adds value to compare directory trees (e.g. partial errors)
				if !tc.compareTrees {
					return
				}
			} else {
				require.NoError(t, err, "Apply failed but shouldn't have")
			}

			// Check if glib-compile-schemas was executed
			if tc.wantGlibMockNotRun {
				require.NoFileExists(t, filepath.Join(temp, glibCompileSchemasRunFile), "glib-compile-schemas was executed but shouldn't have been")
			} else {
				require.FileExists(t, filepath.Join(temp, glibCompileSchemasRunFile), "glib-compile-schemas was not executed but should have been")
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

func TestMockGlibCompileSchemas(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	var mockWritePath, exitMode string

	for len(os.Args) > 0 {
		if os.Args[0] != "--" {
			os.Args = os.Args[1:]
			continue
		}
		mockWritePath = os.Args[1]
		exitMode = os.Args[2]
		break
	}

	if exitMode == "-Exit1-" {
		fmt.Println("EXIT 1 requested in mock")
		os.Exit(1)
	}

	fmt.Println("Mock glib-compile-schemas called")

	err := os.WriteFile(filepath.Join(mockWritePath, glibCompileSchemasRunFile), nil, 0600)
	require.NoError(t, err, "Setup: Couldn't write .ran-compile-schemas file in the test directory")
}

func mockGlibCompileSchemasCmd(t *testing.T, testGoldenPath string) []string {
	t.Helper()

	return []string{"env", "GO_WANT_HELPER_PROCESS=1", os.Args[0], "-test.run=TestMockGlibCompileSchemas", "--", testGoldenPath}
}

func TestMain(m *testing.M) {
	testutils.InstallUpdateFlag()
	flag.Parse()

	m.Run()
}
