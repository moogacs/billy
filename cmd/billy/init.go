package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func runInit(cmd *cobra.Command, args []string) error {
	agent := strings.ToLower(strings.TrimSpace(flagInitAgent))
	if agent == "" {
		agent = "all"
	}
	paths, err := resolveInitPaths()
	if err != nil {
		return err
	}
	if agent == "all" {
		if flagInitUninstall {
			if err := uninstallCursorHook(paths.CursorHooksPath); err != nil {
				return err
			}
			if err := removeManagedBlock(paths.CodexRulesPath, "BILLY_RT_PROXY"); err != nil {
				return err
			}
			if err := removeManagedBlock(paths.ClaudeRulesPath, "BILLY_RT_PROXY"); err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout, "billy: removed agent integrations")
			fmt.Fprintf(os.Stdout, "- cursor: %s\n", paths.CursorHooksPath)
			fmt.Fprintf(os.Stdout, "- codex: %s\n", paths.CodexRulesPath)
			fmt.Fprintf(os.Stdout, "- claude: %s\n", paths.ClaudeRulesPath)
			return nil
		}
		if err := installCursorHook(paths.CursorHooksPath); err != nil {
			return err
		}
		if err := upsertManagedBlock(paths.CodexRulesPath, "BILLY_RT_PROXY", codexRulesBlock()); err != nil {
			return err
		}
		if err := upsertManagedBlock(paths.ClaudeRulesPath, "BILLY_RT_PROXY", claudeRulesBlock()); err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, "billy: installed agent integrations")
		fmt.Fprintf(os.Stdout, "- cursor: %s\n", paths.CursorHooksPath)
		fmt.Fprintf(os.Stdout, "- codex: %s\n", paths.CodexRulesPath)
		fmt.Fprintf(os.Stdout, "- claude: %s\n", paths.ClaudeRulesPath)
		return nil
	}
	switch agent {
	case "cursor":
		if flagInitUninstall {
			if err := uninstallCursorHook(paths.CursorHooksPath); err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, "billy: removed Cursor hook integration (%s)\n", paths.CursorHooksPath)
			return nil
		}
		if err := installCursorHook(paths.CursorHooksPath); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "billy: installed Cursor hook integration (%s)\n", paths.CursorHooksPath)
		return nil
	case "codex":
		if flagInitUninstall {
			if err := removeManagedBlock(paths.CodexRulesPath, "BILLY_RT_PROXY"); err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, "billy: removed Codex AGENTS.md integration (%s)\n", paths.CodexRulesPath)
			return nil
		}
		if err := upsertManagedBlock(paths.CodexRulesPath, "BILLY_RT_PROXY", codexRulesBlock()); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "billy: installed Codex AGENTS.md integration (%s)\n", paths.CodexRulesPath)
		return nil
	case "claude":
		if flagInitUninstall {
			if err := removeManagedBlock(paths.ClaudeRulesPath, "BILLY_RT_PROXY"); err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, "billy: removed Claude CLAUDE.md integration (%s)\n", paths.ClaudeRulesPath)
			return nil
		}
		if err := upsertManagedBlock(paths.ClaudeRulesPath, "BILLY_RT_PROXY", claudeRulesBlock()); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "billy: installed Claude CLAUDE.md integration (%s)\n", paths.ClaudeRulesPath)
		return nil
	default:
		return fmt.Errorf("unknown --agent %q (try all, cursor, codex, claude)", agent)
	}
}

func runProxyHook(cmd *cobra.Command, args []string) error {
	var payload map[string]any
	if err := json.NewDecoder(os.Stdin).Decode(&payload); err != nil {
		// Fail open if hook payload is unreadable.
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"permission": "allow"})
		return nil
	}
	input, _ := payload["input"].(map[string]any)
	command, _ := input["command"].(string)
	if !shouldRewriteShell(command) {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"permission": "allow"})
		return nil
	}
	rewritten := "billy proxy -- " + command
	input["command"] = rewritten
	out := map[string]any{
		"permission":    "allow",
		"updated_input": input,
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}

func shouldRewriteShell(command string) bool {
	c := strings.TrimSpace(command)
	if c == "" {
		return false
	}
	lc := strings.ToLower(c)
	if strings.HasPrefix(lc, "billy proxy ") || lc == "billy proxy" {
		return false
	}
	// Keep v1 safe: only simple single commands, no shell control operators.
	if strings.ContainsAny(c, "|;&><`()$") {
		return false
	}
	fields := strings.Fields(lc)
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "git", "ls", "rg", "grep":
		return true
	case "go":
		return len(fields) > 1 && fields[1] == "test"
	default:
		return false
	}
}

type hooksFile struct {
	Version int                         `json:"version"`
	Hooks   map[string][]map[string]any `json:"hooks"`
}

type initPaths struct {
	CursorHooksPath string
	CodexRulesPath  string
	ClaudeRulesPath string
}

func resolveInitPaths() (initPaths, error) {
	if flagInitProject {
		return initPaths{
			CursorHooksPath: filepath.Join(".cursor", "hooks.json"),
			CodexRulesPath:  "AGENTS.md",
			ClaudeRulesPath: "CLAUDE.md",
		}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return initPaths{}, fmt.Errorf("resolve home directory: %w", err)
	}
	return initPaths{
		CursorHooksPath: filepath.Join(home, ".cursor", "hooks.json"),
		CodexRulesPath:  filepath.Join(home, ".codex", "AGENTS.md"),
		ClaudeRulesPath: filepath.Join(home, ".claude", "CLAUDE.md"),
	}, nil
}

func installCursorHook(hooksPath string) error {
	h, err := readHooks(hooksPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		h = hooksFile{}
	}
	if h.Version == 0 {
		h.Version = 1
	}
	if h.Hooks == nil {
		h.Hooks = map[string][]map[string]any{}
	}
	const event = "preToolUse"
	entry := map[string]any{
		"command": "billy proxy-hook",
		"matcher": "Shell",
	}
	list := h.Hooks[event]
	for _, e := range list {
		if cmd, _ := e["command"].(string); cmd == "billy proxy-hook" {
			return writeHooks(hooksPath, h)
		}
	}
	h.Hooks[event] = append(h.Hooks[event], entry)
	return writeHooks(hooksPath, h)
}

func uninstallCursorHook(hooksPath string) error {
	h, err := readHooks(hooksPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for event, list := range h.Hooks {
		filtered := make([]map[string]any, 0, len(list))
		for _, e := range list {
			if cmd, _ := e["command"].(string); cmd == "billy proxy-hook" {
				continue
			}
			filtered = append(filtered, e)
		}
		if len(filtered) == 0 {
			delete(h.Hooks, event)
			continue
		}
		h.Hooks[event] = filtered
	}
	if len(h.Hooks) == 0 {
		if err := os.Remove(hooksPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return writeHooks(hooksPath, h)
}

func readHooks(path string) (hooksFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return hooksFile{}, err
	}
	var h hooksFile
	if err := json.Unmarshal(b, &h); err != nil {
		return hooksFile{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return h, nil
}

func writeHooks(path string, h hooksFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func upsertManagedBlock(path, id, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	old, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	start := "<!-- " + id + ":START -->"
	end := "<!-- " + id + ":END -->"
	block := start + "\n" + body + "\n" + end + "\n"
	s := string(old)
	if strings.Contains(s, start) && strings.Contains(s, end) {
		i := strings.Index(s, start)
		j := strings.Index(s, end)
		if i >= 0 && j >= i {
			j += len(end)
			s = s[:i] + block + s[j:]
		}
	} else if strings.TrimSpace(s) == "" {
		s = block
	} else {
		if !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		s += "\n" + block
	}
	return os.WriteFile(path, []byte(s), 0o644)
}

func removeManagedBlock(path, id string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	start := "<!-- " + id + ":START -->"
	end := "<!-- " + id + ":END -->"
	s := string(b)
	i := strings.Index(s, start)
	j := strings.Index(s, end)
	if i < 0 || j < 0 || j < i {
		return nil
	}
	j += len(end)
	trimmed := strings.TrimSpace(s[:i] + s[j:])
	if trimmed == "" {
		return os.Remove(path)
	}
	return os.WriteFile(path, []byte(trimmed+"\n"), 0o644)
}

func codexRulesBlock() string {
	return strings.TrimSpace(`
# Billy Runtime Proxy

Use billy runtime compaction for shell-heavy commands to reduce token usage.

Prefer:

- billy proxy -- git status
- billy proxy -- git diff
- billy proxy -- go test ./...
- billy proxy -- rg "pattern" .
- billy gain

When a shell command includes pipelines/control operators, run it normally.
`)
}

func claudeRulesBlock() string {
	return strings.TrimSpace(`
# Billy Runtime Proxy

Use billy runtime compaction for common shell commands to reduce token usage.

Prefer:

- billy proxy -- git status
- billy proxy -- git diff
- billy proxy -- go test ./...
- billy proxy -- rg "pattern" .
- billy gain

Do not rewrite commands containing shell control operators.
`)
}
