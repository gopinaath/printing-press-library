//go:build !windows

package cli

import (
	"os"
	"path/filepath"
)

func tbKnownDocumentsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Documents"), nil
}

func tbResolveFinalPath(p string) (string, error) {
	return filepath.EvalSymlinks(p)
}
