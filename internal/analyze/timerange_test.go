package analyze

import (
	"testing"
	"time"

	"github.com/geekmonkey/billy/internal/model"
)

func TestParseLogTime(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string // RFC3339 of expected instant in UTC
		ok   bool
	}{
		{"2025-01-01T00:01:00Z", "2025-01-01T00:01:00Z", true},
		{"2025-01-01T00:01:00.123456789Z", "2025-01-01T00:01:00.123456789Z", true},
		{"", "", false},
		{"nope", "", false},
	}
	for _, tc := range cases {
		got, ok := ParseLogTime(tc.in)
		if ok != tc.ok {
			t.Fatalf("ParseLogTime(%q) ok=%v want %v", tc.in, ok, tc.ok)
		}
		if !tc.ok {
			continue
		}
		wantT, err := time.Parse(time.RFC3339Nano, tc.want)
		if err != nil {
			t.Fatal(err)
		}
		if !got.Equal(wantT) {
			t.Fatalf("ParseLogTime(%q) = %v want %v", tc.in, got.UTC(), wantT)
		}
	}
}

func TestDayBounds(t *testing.T) {
	t.Parallel()
	loc := time.FixedZone("demo", 5*3600)
	now := time.Date(2025, 3, 15, 14, 30, 0, 0, loc)
	start, end := DayBounds(now)
	if !start.Equal(time.Date(2025, 3, 15, 0, 0, 0, 0, loc)) {
		t.Fatalf("start %v", start)
	}
	if want := start.Add(24 * time.Hour); !end.Equal(want) {
		t.Fatalf("end %v want %v", end, want)
	}
}

func TestMonthBounds(t *testing.T) {
	t.Parallel()
	loc := time.FixedZone("demo", 0)
	now := time.Date(2025, 3, 15, 0, 0, 0, 0, loc)
	start, end := MonthBounds(now)
	if !start.Equal(time.Date(2025, 3, 1, 0, 0, 0, 0, loc)) {
		t.Fatalf("start %v", start)
	}
	if !end.Equal(time.Date(2025, 4, 1, 0, 0, 0, 0, loc)) {
		t.Fatalf("end %v", end)
	}
}

func TestWeekBounds(t *testing.T) {
	t.Parallel()
	loc := time.FixedZone("demo", -5*3600)
	// Sunday 2026-03-29 12:00 → week started Monday 2026-03-23
	now := time.Date(2026, 3, 29, 12, 0, 0, 0, loc)
	start, end := WeekBounds(now)
	wantStart := time.Date(2026, 3, 23, 0, 0, 0, 0, loc)
	wantEnd := time.Date(2026, 3, 30, 0, 0, 0, 0, loc)
	if !start.Equal(wantStart) {
		t.Fatalf("start %v want %v", start, wantStart)
	}
	if !end.Equal(wantEnd) {
		t.Fatalf("end %v want %v", end, wantEnd)
	}
}

func TestFilterReportByWindow(t *testing.T) {
	t.Parallel()
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	rep := model.AnalyzeReport{
		Turns: []model.TurnReport{
			{
				Index:   0,
				Prompt:  "a",
				Answers: []model.AnswerReport{{TurnIndex: 0, Timestamp: "2025-01-01T12:00:00Z", Usage: model.UsageBreakdown{InputTokens: 1}, CostUSD: 1}},
			},
			{
				Index:   1,
				Prompt:  "b",
				Answers: []model.AnswerReport{{TurnIndex: 1, Timestamp: "2025-01-02T00:00:00Z", Usage: model.UsageBreakdown{InputTokens: 2}, CostUSD: 2}},
			},
		},
		Answers: []model.AnswerReport{
			{TurnIndex: 0, Timestamp: "2025-01-01T12:00:00Z", Usage: model.UsageBreakdown{InputTokens: 1}, CostUSD: 1},
			{TurnIndex: 1, Timestamp: "2025-01-02T00:00:00Z", Usage: model.UsageBreakdown{InputTokens: 2}, CostUSD: 2},
		},
	}

	out := FilterReportByWindow(rep, start, end)
	if len(out.Turns) != 1 || len(out.Answers) != 1 {
		t.Fatalf("got turns=%d answers=%d", len(out.Turns), len(out.Answers))
	}
	if out.Turns[0].Index != 0 || out.Turns[0].CostUSD != 1 {
		t.Fatalf("turn0 %+v", out.Turns[0])
	}
	if out.Answers[0].TurnIndex != 0 {
		t.Fatalf("answer turn index %d", out.Answers[0].TurnIndex)
	}
}
