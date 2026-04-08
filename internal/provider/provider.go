package provider

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geekmonkey/billy/internal/anthropic"
	"github.com/geekmonkey/billy/internal/codex"
	"github.com/geekmonkey/billy/internal/cursor"
	"github.com/geekmonkey/billy/internal/fsx"
	"github.com/geekmonkey/billy/internal/model"
)

type dirPick struct {
	path string
	v    model.Vendor
	mod  int64
}

func statPick(path string, v model.Vendor) (dirPick, bool) {
	st, err := os.Stat(path)
	if err != nil {
		return dirPick{}, false
	}
	return dirPick{path, v, st.ModTime().UnixNano()}, true
}

func bestPick(picks []dirPick) (dirPick, bool) {
	if len(picks) == 0 {
		return dirPick{}, false
	}
	best := picks[0]
	for _, p := range picks[1:] {
		if p.mod > best.mod {
			best = p
		}
	}
	return best, true
}

// ProviderName maps resolved vendor to CLI provider.
func ProviderName(v model.Vendor) Name {
	switch v {
	case model.VendorAnthropic:
		return Anthropic
	case model.VendorOpenAI:
		return OpenAI
	case model.VendorCursor:
		return CursorProv
	default:
		return Auto
	}
}

// VendorLabel names the tool for user-facing messages.
func VendorLabel(v model.Vendor) string {
	switch v {
	case model.VendorAnthropic:
		return "Claude Code"
	case model.VendorOpenAI:
		return "Codex"
	case model.VendorCursor:
		return "Cursor"
	default:
		return string(v)
	}
}

// errClaudeProjectMiss means no session JSONL under ~/.claude/projects for the given project paths tried.
var errClaudeProjectMiss = errors.New("no Claude Code session for this project directory")

// Name matches CLI --provider values.
type Name string

const (
	Auto       Name = "auto"
	Anthropic  Name = "anthropic"
	OpenAI     Name = "openai"
	CursorProv Name = "cursor"
)

// ParseProviderFlag maps CLI --provider values and common aliases to a Name.
// cc → anthropic (Claude Code); codex → openai.
func ParseProviderFlag(s string) (Name, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "", "auto":
		return Auto, nil
	case "anthropic":
		return Anthropic, nil
	case "cc", "claude", "claude-code":
		return Anthropic, nil
	case "openai":
		return OpenAI, nil
	case "codex":
		return OpenAI, nil
	case "cursor":
		return CursorProv, nil
	default:
		return "", fmt.Errorf("unknown provider %q (try auto, anthropic|cc, openai|codex, cursor)", s)
	}
}

// ReadSession loads normalized events for a file or directory path.
func ReadSession(p Name, path string) (model.Vendor, []model.NormalizedEvent, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", nil, err
	}

	var v model.Vendor
	if p == Auto {
		v = DetectVendor(path, st.IsDir())
	} else {
		v = model.Vendor(p)
	}

	switch v {
	case model.VendorAnthropic:
		if st.IsDir() {
			return "", nil, fmt.Errorf("anthropic: pass a session .jsonl file, not a directory")
		}
		ev, err := anthropic.ReadEvents(path)
		return model.VendorAnthropic, ev, err
	case model.VendorOpenAI:
		if st.IsDir() {
			return "", nil, fmt.Errorf("openai/codex: pass a session .jsonl file, not a directory")
		}
		ev, err := codex.ReadEvents(path)
		return model.VendorOpenAI, ev, err
	case model.VendorCursor:
		if st.IsDir() {
			return "", nil, fmt.Errorf("cursor: pass a .jsonl transcript or .vscdb / store.db file")
		}
		if strings.HasSuffix(strings.ToLower(path), ".vscdb") || strings.HasSuffix(strings.ToLower(filepath.Base(path)), "store.db") {
			ev, err := cursor.ReadEventsFromSQLite(path)
			return model.VendorCursor, ev, err
		}
		ev, err := cursor.ReadEvents(path)
		return model.VendorCursor, ev, err
	default:
		return "", nil, fmt.Errorf("unknown provider for path %q", path)
	}
}

// DetectVendor picks a vendor from path heuristics when p is auto.
func DetectVendor(path string, isDir bool) model.Vendor {
	np := strings.ToLower(filepath.ToSlash(path))
	switch {
	case strings.Contains(np, "/.claude/") || strings.Contains(np, "/.config/claude/"):
		return model.VendorAnthropic
	case strings.Contains(np, "/.codex/") || strings.HasPrefix(np, strings.ToLower(codex.DefaultHome()+"/")):
		return model.VendorOpenAI
	case strings.Contains(np, "/.cursor/") || strings.Contains(np, "cursor"):
		return model.VendorCursor
	default:
		if !isDir && strings.HasSuffix(np, ".jsonl") {
			// Default ambiguous jsonl to anthropic (Claude Code is common).
			return model.VendorAnthropic
		}
	}
	return model.VendorAnthropic
}

func candidateProjectDirs(projectAbs string) []string {
	abs, err := filepath.Abs(filepath.Clean(projectAbs))
	if err != nil {
		return []string{filepath.Clean(projectAbs)}
	}
	seen := make(map[string]struct{})
	var out []string
	add := func(p string) {
		p = filepath.Clean(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	add(abs)
	if sym, err := filepath.EvalSymlinks(abs); err == nil && sym != "" {
		sym = filepath.Clean(sym)
		add(sym)
		if symAbs, err := filepath.Abs(sym); err == nil {
			add(symAbs)
		}
	}
	return out
}

// resolveAnthropicFromProjectDirs uses ~/.claude/projects/<encoded path> (and symlink variants) only.
func resolveAnthropicFromProjectDirs(projectDir string, includeAgents bool) (string, error) {
	var collected []string
	for _, d := range candidateProjectDirs(projectDir) {
		files, err := anthropic.ListSessionFiles(d, includeAgents)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", err
		}
		collected = append(collected, files...)
	}
	if len(collected) > 0 {
		return fsx.LatestByModTime(collected), nil
	}
	return "", errClaudeProjectMiss
}

// ResolveDirToSession maps a directory to a concrete log path and vendor.
// usedFallback is true when the result was not the project-scoped Claude folder for that directory
// (or when using Codex/Cursor directory discovery, or auto picked a non-project log).
func ResolveDirToSession(p Name, projectDir string, includeAgents bool) (sessionPath string, vendor model.Vendor, usedFallback bool, err error) {
	switch p {
	case OpenAI:
		sess, err := codex.LatestSessionJSONL()
		if err != nil {
			return "", "", false, err
		}
		return sess, model.VendorOpenAI, true, nil

	case CursorProv:
		sess, err := cursor.LatestWorkspaceStateDB()
		if err != nil {
			return "", "", false, err
		}
		return sess, model.VendorCursor, true, nil

	case Anthropic:
		sess, err := resolveAnthropicFromProjectDirs(projectDir, includeAgents)
		if err == nil {
			return sess, model.VendorAnthropic, false, nil
		}
		if !errors.Is(err, errClaudeProjectMiss) {
			return "", "", false, err
		}
		fallback, err := anthropic.LatestSessionAnywhere(includeAgents)
		if err != nil {
			return "", "", false, fmt.Errorf("no Claude Code session for project %q: %w", projectDir, err)
		}
		return fallback, model.VendorAnthropic, true, nil

	case Auto:
		sess, err := resolveAnthropicFromProjectDirs(projectDir, includeAgents)
		if err == nil {
			return sess, model.VendorAnthropic, false, nil
		}
		if !errors.Is(err, errClaudeProjectMiss) {
			return "", "", false, err
		}
		var picks []dirPick
		if a, err := anthropic.LatestSessionAnywhere(includeAgents); err == nil {
			if pk, ok := statPick(a, model.VendorAnthropic); ok {
				picks = append(picks, pk)
			}
		}
		if c, err := codex.LatestSessionJSONL(); err == nil {
			if pk, ok := statPick(c, model.VendorOpenAI); ok {
				picks = append(picks, pk)
			}
		}
		if cu, err := cursor.LatestWorkspaceStateDB(); err == nil {
			if pk, ok := statPick(cu, model.VendorCursor); ok {
				picks = append(picks, pk)
			}
		}
		best, ok := bestPick(picks)
		if !ok {
			return "", "", false, fmt.Errorf("no local agent logs under default paths (Claude: ~/.claude and ~/.config/claude, Codex: %s/sessions, Cursor: %s)",
				codex.DefaultHome(), cursor.WorkspaceStorageRoot())
		}
		return best.path, best.v, true, nil

	default:
		return "", "", false, fmt.Errorf("unknown provider %q", p)
	}
}
