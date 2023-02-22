package testutils

import (
	"flag"
	"path/filepath"
	"strings"
	"testing"
)

var update bool

// NormalizeGoldenName returns the name of the golden file with illegal Windows
// characters replaced or removed.
func NormalizeGoldenName(t *testing.T, name string) string {
	t.Helper()

	name = strings.ReplaceAll(name, `\`, "_")
	name = strings.ReplaceAll(name, ":", "")
	name = strings.ToLower(name)
	return name
}

// TestFamilyPath returns the path of the dir for storing fixtures and other files related to the test.
func TestFamilyPath(t *testing.T) string {
	t.Helper()

	// Ensures that only the name of the parent test is used.
	super, _, _ := strings.Cut(t.Name(), "/")

	return filepath.Join("testdata", super)
}

// GoldenPath returns the golden path for the provided test.
func GoldenPath(t *testing.T) string {
	t.Helper()

	path := filepath.Join(TestFamilyPath(t), "golden")
	_, sub, found := strings.Cut(t.Name(), "/")
	if found {
		path = filepath.Join(path, NormalizeGoldenName(t, sub))
	}

	return path
}

// InstallUpdateFlag install an update flag referenced in this package.
// The flags need to be parsed before running the tests.
func InstallUpdateFlag() {
	flag.BoolVar(&update, "update", false, "update golden files")
}

// Update returns true if the update flag was set, false otherwise.
func Update() bool {
	return update
}
