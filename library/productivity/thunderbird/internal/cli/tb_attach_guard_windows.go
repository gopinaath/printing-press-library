//go:build windows

package cli

import (
	"strings"

	"golang.org/x/sys/windows"
)

func tbKnownDocumentsDir() (string, error) {
	return windows.KnownFolderPath(windows.FOLDERID_Documents, 0)
}

func tbResolveFinalPath(p string) (string, error) {
	name, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return "", err
	}
	h, err := windows.CreateFile(name, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
	if err != nil {
		return "", err
	}
	final := windows.UTF16ToString(buf[:n])
	if strings.HasPrefix(final, `\\?\UNC\`) {
		return `\\` + final[len(`\\?\UNC\`):], nil
	}
	return strings.TrimPrefix(final, `\\?\`), nil
}
