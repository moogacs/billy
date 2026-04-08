package provider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geekmonkey/billy/internal/anthropic"
	"github.com/geekmonkey/billy/internal/model"
)

func TestParseProviderFlag_aliases(t *testing.T) {
	tests := []struct {
		in   string
		want Name
	}{
		{"cc", Anthropic},
		{"CC", Anthropic},
		{"claude-code", Anthropic},
		{"codex", OpenAI},
		{"anthropic", Anthropic},
		{"openai", OpenAI},
		{"cursor", CursorProv},
		{"auto", Auto},
		{"", Auto},
	}
	for _, tc := range tests {
		got, err := ParseProviderFlag(tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("ParseProviderFlag(%q) = %v, %v want %v, nil", tc.in, got, err, tc.want)
		}
	}
	if _, err := ParseProviderFlag("nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveDirToSession_openaiDirUsesCodexSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sessDir := filepath.Join(home, ".codex", "sessions", "2026", "01", "01")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(sessDir, "rollout.jsonl")
	if err := os.WriteFile(f, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(home, "myapp")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	got, v, fb, err := ResolveDirToSession(OpenAI, proj, false)
	if err != nil {
		t.Fatal(err)
	}
	if v != model.VendorOpenAI {
		t.Fatalf("vendor %v", v)
	}
	if !fb {
		t.Fatal("expected directory discovery")
	}
	if got != f {
		t.Fatalf("got %q want %q", got, f)
	}
}

func TestResolveDirToSession_autoNoAgentLogs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_, _, _, err := ResolveDirToSession(Auto, dir, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no local agent") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveDirToSession_anthropicNoClaudeFolder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_, _, _, err := ResolveDirToSession(Anthropic, dir, false)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveDirToSession_anthropicResolvesLatest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	proj := filepath.Join(home, "src", "demo")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	projAbs, err := filepath.Abs(proj)
	if err != nil {
		t.Fatal(err)
	}
	enc := anthropic.EncodeProjectPath(projAbs)
	sessDir := filepath.Join(home, ".claude", "projects", enc)
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(sessDir, "a.jsonl")
	newPath := filepath.Join(sessDir, "b.jsonl")
	if err := os.WriteFile(oldPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldT := mustParseTime(t, "2020-01-01T00:00:00Z")
	newT := mustParseTime(t, "2030-01-01T00:00:00Z")
	if err := os.Chtimes(oldPath, oldT, oldT); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newPath, newT, newT); err != nil {
		t.Fatal(err)
	}

	got, v, usedFallback, err := ResolveDirToSession(Anthropic, projAbs, false)
	if err != nil {
		t.Fatal(err)
	}
	if usedFallback {
		t.Fatal("did not expect global fallback")
	}
	if v != model.VendorAnthropic {
		t.Fatalf("vendor %v", v)
	}
	if filepath.Base(got) != "b.jsonl" {
		t.Fatalf("want latest b.jsonl, got %q", got)
	}
}

func TestResolveDirToSession_anthropicResolvesNestedSessionsDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	proj := filepath.Join(home, "app")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	projAbs, err := filepath.Abs(proj)
	if err != nil {
		t.Fatal(err)
	}
	enc := anthropic.EncodeProjectPath(projAbs)
	encDir := filepath.Join(home, ".claude", "projects", enc)
	nested := filepath.Join(encDir, "sessions")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(nested, "uuid.jsonl")
	if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, v, fb, err := ResolveDirToSession(Anthropic, projAbs, false)
	if err != nil {
		t.Fatal(err)
	}
	if fb || v != model.VendorAnthropic {
		t.Fatalf("fb=%v v=%v", fb, v)
	}
	if got != p {
		t.Fatalf("got %q want %q", got, p)
	}
}

func TestResolveDirToSession_fallsBackToGlobalLatest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	wrongProj := filepath.Join(home, "unrelated", "repo")
	if err := os.MkdirAll(wrongProj, 0o755); err != nil {
		t.Fatal(err)
	}
	wrongAbs, err := filepath.Abs(wrongProj)
	if err != nil {
		t.Fatal(err)
	}

	otherEnc := "-somewhere-else"
	sessDir := filepath.Join(home, ".claude", "projects", otherEnc)
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(sessDir, "global.jsonl")
	if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	newT := mustParseTime(t, "2035-06-01T00:00:00Z")
	if err := os.Chtimes(p, newT, newT); err != nil {
		t.Fatal(err)
	}

	got, v, usedFallback, err := ResolveDirToSession(Auto, wrongAbs, false)
	if err != nil {
		t.Fatal(err)
	}
	if !usedFallback {
		t.Fatal("expected global fallback")
	}
	if v != model.VendorAnthropic {
		t.Fatalf("vendor %v", v)
	}
	if got != p {
		t.Fatalf("got %q want %q", got, p)
	}
}

func TestResolveDirToSession_autoPicksNewerVendor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	wrongProj := filepath.Join(home, "orphan")
	if err := os.MkdirAll(wrongProj, 0o755); err != nil {
		t.Fatal(err)
	}
	wrongAbs, err := filepath.Abs(wrongProj)
	if err != nil {
		t.Fatal(err)
	}

	claudeEnc := "-c1"
	clDir := filepath.Join(home, ".claude", "projects", claudeEnc)
	if err := os.MkdirAll(clDir, 0o755); err != nil {
		t.Fatal(err)
	}
	clFile := filepath.Join(clDir, "old.jsonl")
	if err := os.WriteFile(clFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(clFile, mustParseTime(t, "2020-01-01T00:00:00Z"), mustParseTime(t, "2020-01-01T00:00:00Z")); err != nil {
		t.Fatal(err)
	}

	cxDir := filepath.Join(home, ".codex", "sessions", "d")
	if err := os.MkdirAll(cxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cxFile := filepath.Join(cxDir, "new.jsonl")
	if err := os.WriteFile(cxFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(cxFile, mustParseTime(t, "2040-01-01T00:00:00Z"), mustParseTime(t, "2040-01-01T00:00:00Z")); err != nil {
		t.Fatal(err)
	}

	got, v, _, err := ResolveDirToSession(Auto, wrongAbs, false)
	if err != nil {
		t.Fatal(err)
	}
	if v != model.VendorOpenAI || got != cxFile {
		t.Fatalf("want Codex %q got %v %q", cxFile, v, got)
	}
}

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	tt, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return tt
}
