package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geekmonkey/billy/internal/fsx"
)

// CollectSessionJSONLPaths returns every *.jsonl under $CODEX_HOME/sessions (recursive).
func CollectSessionJSONLPaths() []string {
	root := filepath.Join(DefaultHome(), "sessions")
	var out []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".jsonl") {
			out = append(out, path)
		}
		return nil
	})
	return out
}

// LatestSessionJSONL returns the most recently modified Codex session file.
func LatestSessionJSONL() (string, error) {
	paths := CollectSessionJSONLPaths()
	if len(paths) == 0 {
		return "", fmt.Errorf("no Codex session .jsonl under %s/sessions", DefaultHome())
	}
	return fsx.LatestByModTime(paths), nil
}
