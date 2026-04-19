package proxy

import (
	"fmt"
	"path/filepath"
	"strings"
)

func Compact(args []string, raw string) string {
	if len(args) == 0 {
		return compactGeneric(raw, 120)
	}
	cmd := filepath.Base(args[0])
	switch cmd {
	case "git":
		return compactGit(args[1:], raw)
	case "go":
		if len(args) > 1 && args[1] == "test" {
			return compactGoTest(raw)
		}
	case "ls":
		return compactLS(raw)
	case "rg", "grep":
		return compactHead(raw, 80)
	}
	return compactGeneric(raw, 120)
}

func compactGit(args []string, raw string) string {
	if len(args) == 0 {
		return compactGeneric(raw, 120)
	}
	switch args[0] {
	case "status":
		return compactGitStatus(raw)
	case "diff":
		return compactGitDiff(raw)
	case "log":
		return compactHead(raw, 40)
	default:
		return compactGeneric(raw, 120)
	}
}

func compactGitStatus(raw string) string {
	lines := splitNonEmpty(raw)
	if len(lines) == 0 {
		return "git status: clean"
	}
	branch := ""
	changed, untracked := 0, 0
	var sample []string
	for _, l := range lines {
		if strings.HasPrefix(l, "## ") && branch == "" {
			branch = strings.TrimPrefix(l, "## ")
			continue
		}
		if strings.HasPrefix(l, "?? ") {
			untracked++
			if len(sample) < 10 {
				sample = append(sample, strings.TrimSpace(strings.TrimPrefix(l, "?? ")))
			}
			continue
		}
		if len(l) >= 2 {
			changed++
			if len(sample) < 10 {
				sample = append(sample, strings.TrimSpace(l[3:]))
			}
		}
	}
	var b strings.Builder
	if branch != "" {
		b.WriteString("branch: ")
		b.WriteString(branch)
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf("changed: %d, untracked: %d\n", changed, untracked))
	if len(sample) > 0 {
		b.WriteString("files:\n")
		for _, s := range sample {
			b.WriteString("- ")
			b.WriteString(s)
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func compactGitDiff(raw string) string {
	lines := strings.Split(raw, "\n")
	added, removed, hunks := 0, 0, 0
	for _, l := range lines {
		if strings.HasPrefix(l, "@@") {
			hunks++
			continue
		}
		if strings.HasPrefix(l, "+++") || strings.HasPrefix(l, "---") {
			continue
		}
		if strings.HasPrefix(l, "+") {
			added++
		}
		if strings.HasPrefix(l, "-") {
			removed++
		}
	}
	return fmt.Sprintf("git diff summary: hunks=%d +%d -%d", hunks, added, removed)
}

func compactGoTest(raw string) string {
	lines := strings.Split(raw, "\n")
	var keep []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		if strings.Contains(t, "--- FAIL:") ||
			strings.HasPrefix(t, "FAIL") ||
			strings.Contains(t, "panic:") ||
			strings.HasPrefix(t, "ok  ") ||
			strings.HasPrefix(t, "?   ") {
			keep = append(keep, l)
		}
	}
	if len(keep) == 0 {
		return compactHead(raw, 40)
	}
	return strings.Join(keep, "\n")
}

func compactLS(raw string) string {
	lines := splitNonEmpty(raw)
	total := len(lines)
	if total == 0 {
		return "empty directory"
	}
	if total > 30 {
		lines = lines[:30]
	}
	return fmt.Sprintf("entries: %d\n%s", total, strings.Join(lines, "\n"))
}

func compactGeneric(raw string, maxLines int) string {
	lines := splitNonEmpty(raw)
	if len(lines) == 0 {
		return ""
	}
	out := make([]string, 0, len(lines))
	prev := ""
	dup := 0
	flush := func() {
		if prev == "" {
			return
		}
		if dup > 1 {
			out = append(out, fmt.Sprintf("%s (x%d)", prev, dup))
		} else {
			out = append(out, prev)
		}
	}
	for _, l := range lines {
		if l == prev {
			dup++
			continue
		}
		flush()
		prev = l
		dup = 1
	}
	flush()
	if len(out) > maxLines {
		out = out[:maxLines]
		out = append(out, fmt.Sprintf("... truncated %d lines", len(lines)-maxLines))
	}
	return strings.Join(out, "\n")
}

func compactHead(raw string, maxLines int) string {
	lines := splitNonEmpty(raw)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines = append(lines, "... truncated")
	}
	return strings.Join(lines, "\n")
}

func splitNonEmpty(raw string) []string {
	parts := strings.Split(raw, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimRight(p, " \t")
		if strings.TrimSpace(t) == "" {
			continue
		}
		out = append(out, t)
	}
	return out
}
