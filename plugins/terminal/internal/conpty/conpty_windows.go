package conpty

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/plugins/terminal/internal/library"
)

//go:embed mygo-plugin.json
var natives []byte

// Path returns the conpty.dll to load, with OpenConsole.exe beside it,
// where conpty.dll looks for it: among the app's resources, next to the
// executable or in the user's cache, as library.FindNamed looks, the file
// $MYGO_CONPTY names first. Programs that are not packaged apps download
// both into the cache when they are missing.
func Path() (string, error) {
	dll, err := library.FindNamed(natives, "conpty", "MYGO_CONPTY")
	if err != nil {
		return "", err
	}
	if os.Getenv("MYGO_CONPTY") != "" {
		return dll, nil // as named, with its host where its owner put it
	}
	host := filepath.Join(filepath.Dir(dll), "OpenConsole.exe")
	if _, err := os.Stat(host); err == nil {
		return dll, nil
	}
	// In the user's cache each file has a folder of its own, by its
	// SHA-256: OpenConsole.exe joins conpty.dll in its.
	exe, err := library.FindNamed(natives, "openconsole", "MYGO_OPENCONSOLE")
	if err != nil {
		return "", err
	}
	if err := copyFile(exe, host); err != nil {
		return "", fmt.Errorf("terminal: putting OpenConsole.exe beside conpty.dll: %w", err)
	}
	return dll, nil
}

// copyFile copies src to dst, whole or not at all.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".OpenConsole-*.exe")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), dst)
	}
	if err != nil {
		os.Remove(tmp.Name())
		if _, serr := os.Stat(dst); serr == nil {
			return nil // another process put it there first
		}
	}
	return err
}
