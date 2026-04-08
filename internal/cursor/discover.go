package cursor

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/geekmonkey/billy/internal/fsx"
)

// WorkspaceStorageRoot is Cursor's workspaceStorage directory (workspace-scoped DBs).
func WorkspaceStorageRoot() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = os.Getenv("HOME")
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Cursor", "User", "workspaceStorage")
	case "windows":
		if ad := os.Getenv("APPDATA"); ad != "" {
			return filepath.Join(ad, "Cursor", "User", "workspaceStorage")
		}
		return filepath.Join(home, "AppData", "Roaming", "Cursor", "User", "workspaceStorage")
	default:
		return filepath.Join(home, ".config", "Cursor", "User", "workspaceStorage")
	}
}

// CollectWorkspaceStateDBs finds state.vscdb and store.db under workspaceStorage.
func CollectWorkspaceStateDBs() []string {
	root := WorkspaceStorageRoot()
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return nil
	}
	var out []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		n := strings.ToLower(d.Name())
		if n == "state.vscdb" || n == "store.db" {
			out = append(out, path)
		}
		return nil
	})
	return out
}

// LatestWorkspaceStateDB returns the newest Cursor SQLite store by mtime.
func LatestWorkspaceStateDB() (string, error) {
	paths := CollectWorkspaceStateDBs()
	if len(paths) == 0 {
		return "", fmt.Errorf("no Cursor state.vscdb or store.db under %s", WorkspaceStorageRoot())
	}
	return fsx.LatestByModTime(paths), nil
}
