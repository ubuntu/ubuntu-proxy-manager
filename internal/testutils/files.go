package testutils

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/termie/go-shutil"
)

// MakeReadOnly makes dest read only and restore permission on cleanup.
func MakeReadOnly(t *testing.T, dest string) {
	t.Helper()

	// Get current dest permissions
	fi, err := os.Stat(dest)
	require.NoError(t, err, "Cannot stat %s", dest)
	mode := fi.Mode()

	var perms fs.FileMode = 0444
	if fi.IsDir() {
		perms = 0555
	}
	err = os.Chmod(dest, perms)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, err := os.Stat(dest)
		if errors.Is(err, os.ErrNotExist) {
			return
		}

		err = os.Chmod(dest, mode)
		require.NoError(t, err)
	})
}

// Chmod applies the given mode to dest and restores permissions on cleanup.
func Chmod(t *testing.T, dest string, mode os.FileMode) {
	t.Helper()

	// Get current dest permissions
	fi, err := os.Stat(dest)
	require.NoError(t, err, "Cannot stat %s", dest)
	initialMode := fi.Mode()

	err = os.Chmod(dest, mode)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, err := os.Stat(dest)
		if errors.Is(err, os.ErrNotExist) {
			return
		}

		err = os.Chmod(dest, initialMode)
		require.NoError(t, err)
	})
}

const fileForEmptyDir = ".empty"

// CompareTreesWithFiltering allows comparing a goldPath directory to p. Those can be updated via the dedicated flag.
func CompareTreesWithFiltering(t *testing.T, p, goldPath string, update bool) {
	t.Helper()

	// Update golden file
	if update {
		t.Logf("updating golden file %s", goldPath)
		require.NoError(t, os.RemoveAll(goldPath), "Cannot remove target golden directory")

		// check the source directory exists before trying to copy it
		info, err := os.Stat(p)
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		require.NoErrorf(t, err, "Error on checking %q", p)

		if !info.IsDir() {
			// copy file
			// #nosec G304 - path controlled by test
			data, err := os.ReadFile(p)
			require.NoError(t, err, "Cannot read new generated file file %s", p)
			require.NoError(t, os.WriteFile(goldPath, data, info.Mode()), "Cannot write golden file")
		} else {
			require.NoError(t,
				shutil.CopyTree(
					p, goldPath,
					&shutil.CopyTreeOptions{Symlinks: true, CopyFunction: shutil.Copy}),
				"Can’t update golden directory")
			require.NoError(t, addEmptyMarker(goldPath), "Cannot create empty file in empty directories")
		}
	}

	var gotContent map[string]treeAttrs
	if _, err := os.Stat(p); err == nil {
		gotContent, err = treeContentAndAttrs(t, p, []byte("GVariant"))
		if err != nil {
			t.Fatalf("No generated content: %v", err)
		}
	}

	var goldContent map[string]treeAttrs
	if _, err := os.Stat(goldPath); err == nil {
		goldContent, err = treeContentAndAttrs(t, goldPath, nil)
		if err != nil {
			t.Fatalf("No golden directory found: %v", err)
		}
	}
	assert.Equal(t, goldContent, gotContent, "got and expected content differs")
}

// addEmptyMarker adds to any empty directory, fileForEmptyDir to it.
// That allows git to commit it.
func addEmptyMarker(p string) error {
	err := filepath.WalkDir(p, func(path string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !de.IsDir() {
			return nil
		}

		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			// #nosec G304 - path controlled by test
			f, err := os.Create(filepath.Join(path, fileForEmptyDir))
			if err != nil {
				return err
			}
			_ = f.Close()
		}
		return nil
	})

	return err
}

// treeAttrs are the attributes to take into consideration when comparing each file.
type treeAttrs struct {
	content    string
	path       string
	executable bool
}

// treeContentAndAttrs builds a recursive file list of dir with their content and other attributes.
// It can ignore files starting with ignoreHeaders.
func treeContentAndAttrs(t *testing.T, dir string, ignoreHeaders []byte) (map[string]treeAttrs, error) {
	t.Helper()

	r := make(map[string]treeAttrs)

	err := filepath.WalkDir(dir, func(path string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Ignore markers for empty directories
		if filepath.Base(path) == fileForEmptyDir {
			return nil
		}

		content := ""
		info, err := os.Stat(path)
		require.NoError(t, err, "Cannot stat %s", path)
		if !de.IsDir() {
			// #nosec G304 - path controlled by test
			d, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			// ignore given header
			if ignoreHeaders != nil && bytes.HasPrefix(d, ignoreHeaders) {
				return nil
			}
			content = string(d)
		}
		trimmedPath := strings.TrimPrefix(path, dir)
		r[trimmedPath] = treeAttrs{content, strings.TrimPrefix(path, dir), info.Mode()&0111 != 0}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return r, nil
}
