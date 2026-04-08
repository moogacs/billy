package anthropic

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geekmonkey/billy/internal/fsx"
)

// EncodeProjectPath maps an absolute project directory to Claude Code's folder name.
func EncodeProjectPath(abs string) string {
	p := filepath.Clean(abs)
	if p == string(os.PathSeparator) || p == "." {
		return "-"
	}
	if vol := filepath.VolumeName(p); vol != "" {
		p = p[len(vol):]
	}
	p = strings.ReplaceAll(p, string(os.PathSeparator), "-")
	if !strings.HasPrefix(p, "-") {
		p = "-" + p
	}
	return p
}

// SessionDir returns the directory containing JSONL sessions for a project, or "" if missing.
func SessionDir(projectAbs string) string {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = os.Getenv("HOME")
	}
	enc := EncodeProjectPath(projectAbs)
	candidates := []string{
		filepath.Join(home, ".claude", "projects", enc),
		filepath.Join(home, ".config", "claude", "projects", enc),
	}
	for _, d := range candidates {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d
		}
	}
	return ""
}

// SessionFilesInProjectEncDir lists *.jsonl in a project slug directory and in its sessions/ subfolder.
func SessionFilesInProjectEncDir(encDir string, includeAgents bool) ([]string, error) {
	if _, err := os.Stat(encDir); err != nil {
		return nil, err
	}
	var out []string
	for _, part := range []string{encDir, filepath.Join(encDir, "sessions")} {
		fs, err := SessionFilesInDir(part, includeAgents)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		out = append(out, fs...)
	}
	return out, nil
}

// ListSessionFiles returns main session JSONL paths (excludes agent-*.jsonl unless includeAgents).
func ListSessionFiles(projectAbs string, includeAgents bool) ([]string, error) {
	dir := SessionDir(projectAbs)
	if dir == "" {
		return nil, os.ErrNotExist
	}
	return SessionFilesInProjectEncDir(dir, includeAgents)
}

// SessionFilesInDir lists *.jsonl in a single session directory.
func SessionFilesInDir(dir string, includeAgents bool) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		if strings.HasPrefix(name, "agent-") && !includeAgents {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	return out, nil
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	if h == "" {
		h = os.Getenv("HOME")
	}
	return h
}

// DefaultProjectRoots returns Claude Code project storage dirs that exist:
// ~/.claude/projects and ~/.config/claude/projects.
func DefaultProjectRoots() []string {
	home := homeDir()
	if home == "" {
		return nil
	}
	candidates := []string{
		filepath.Join(home, ".claude", "projects"),
		filepath.Join(home, ".config", "claude", "projects"),
	}
	var out []string
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			out = append(out, p)
		}
	}
	return out
}

func globalClaudeSessionDirs() []string {
	home := homeDir()
	if home == "" {
		return nil
	}
	candidates := []string{
		filepath.Join(home, ".claude", "sessions"),
		filepath.Join(home, ".config", "claude", "sessions"),
	}
	var out []string
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			out = append(out, p)
		}
	}
	return out
}

func walkClaudeJSONL(root string, includeAgents bool) []string {
	var out []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".jsonl") {
			return nil
		}
		base := filepath.Base(path)
		if strings.EqualFold(base, "history.jsonl") {
			return nil
		}
		if strings.HasPrefix(base, "agent-") && !includeAgents {
			return nil
		}
		out = append(out, path)
		return nil
	})
	return out
}

// AllAnthropicSessionPaths returns every known Claude Code session JSONL: per-project trees
// (including projects/<slug>/sessions/) plus global ~/.claude/sessions and ~/.config/claude/sessions.
func AllAnthropicSessionPaths(includeAgents bool) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(paths []string) {
		for _, p := range paths {
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}

	for _, root := range DefaultProjectRoots() {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			encDir := filepath.Join(root, e.Name())
			fs, err := SessionFilesInProjectEncDir(encDir, includeAgents)
			if err != nil {
				continue
			}
			add(fs)
		}
	}

	for _, g := range globalClaudeSessionDirs() {
		add(walkClaudeJSONL(g, includeAgents))
	}
	return out
}

// LatestSessionAnywhere picks the newest *.jsonl from AllAnthropicSessionPaths.
func LatestSessionAnywhere(includeAgents bool) (string, error) {
	paths := AllAnthropicSessionPaths(includeAgents)
	if len(paths) == 0 {
		return "", fmt.Errorf("no Claude Code session .jsonl under ~/.claude (projects, sessions) or ~/.config/claude")
	}
	return fsx.LatestByModTime(paths), nil
}
