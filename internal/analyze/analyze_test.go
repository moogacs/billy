package analyze

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/geekmonkey/billy/internal/anthropic"
	"github.com/geekmonkey/billy/internal/codex"
	"github.com/geekmonkey/billy/internal/cursor"
	"github.com/geekmonkey/billy/internal/model"
	"github.com/geekmonkey/billy/internal/pricing"
)

func TestBuildReport_anthropicGolden(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..")
	pt, err := pricing.Load(filepath.Join(root, "testdata", "pricing_test.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	ev, err := anthropic.ReadEvents(filepath.Join(root, "testdata", "anthropic", "min.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	rep := BuildReport("min.jsonl", model.VendorAnthropic, ev, pt)
	if len(rep.Turns) != 2 {
		t.Fatalf("turns %d", len(rep.Turns))
	}
	const eps = 1e-6
	if got := rep.Turns[0].CostUSD; got < 18-eps || got > 18+eps {
		t.Fatalf("turn0 cost got %v want 18", got)
	}
	if got := rep.Turns[1].CostUSD; got < 6-eps || got > 6+eps {
		t.Fatalf("turn1 cost got %v want 6", got)
	}
	if len(rep.Answers) != 2 {
		t.Fatalf("answers %d", len(rep.Answers))
	}
}

func TestBuildReport_codexGolden(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..")
	pt, err := pricing.Load(filepath.Join(root, "testdata", "pricing_test.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	ev, err := codex.ReadEvents(filepath.Join(root, "testdata", "codex", "min.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	rep := BuildReport("min.jsonl", model.VendorOpenAI, ev, pt)
	if len(rep.Turns) != 1 {
		t.Fatalf("turns %d", len(rep.Turns))
	}
	want := 0.15 + 0.30 // 1M in @0.15, 0.5M out @0.60
	const eps = 1e-6
	if got := rep.Turns[0].CostUSD; got < want-eps || got > want+eps {
		t.Fatalf("turn0 cost got %v want %v", got, want)
	}
}

func TestBuildReport_cursorGolden(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..")
	pt, err := pricing.Load(filepath.Join(root, "testdata", "pricing_test.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	ev, err := cursor.ReadEvents(filepath.Join(root, "testdata", "cursor", "min.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	rep := BuildReport("min.jsonl", model.VendorCursor, ev, pt)
	want := 0.15 + 0.30
	const eps = 1e-6
	if got := rep.Turns[0].CostUSD; got < want-eps || got > want+eps {
		t.Fatalf("cursor turn0 cost got %v want %v", got, want)
	}
}

func TestSumAnswers_matchesTurnCosts(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..")
	pt, err := pricing.Load(filepath.Join(root, "testdata", "pricing_test.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	ev, err := anthropic.ReadEvents(filepath.Join(root, "testdata", "anthropic", "min.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	rep := BuildReport("min.jsonl", model.VendorAnthropic, ev, pt)
	cost, _, ans, _ := SumAnswers(rep)
	var want float64
	for _, tr := range rep.Turns {
		want += tr.CostUSD
	}
	const eps = 1e-6
	if cost < want-eps || cost > want+eps {
		t.Fatalf("sum answers cost %v want %v", cost, want)
	}
	if ans != len(rep.Answers) {
		t.Fatalf("answers %d want %d", ans, len(rep.Answers))
	}
}
