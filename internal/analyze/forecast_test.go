package analyze

import (
	"testing"
	"time"

	"github.com/geekmonkey/billy/internal/model"
)

func TestForecastWeeklyLimit(t *testing.T) {
	t.Parallel()
	loc := time.UTC
	weekStart := time.Date(2026, 3, 23, 0, 0, 0, 0, loc) // Monday
	now := weekStart.Add(10 * time.Hour)

	answers := []model.AnswerReport{
		{Timestamp: weekStart.Add(1 * time.Hour).Format(time.RFC3339), CostUSD: 100, Usage: model.UsageBreakdown{InputTokens: 1000}},
	}

	// Force week by using `now` — WeekBounds(now) should match weekStart for this instant
	_, wEnd := WeekBounds(now)
	if !wEnd.After(now) {
		t.Fatal("bad fixture")
	}

	f := ForecastWeeklyLimit(answers, now, 500, ForecastUSD)
	if f.AlreadyExceeded {
		t.Fatal("unexpected exceeded")
	}
	if f.HitAt == nil {
		t.Fatalf("want hit time, got reason %q", f.NoForecastReason)
	}
	// used 100 / max(10,1) = 10/h; 400 remaining -> 40h
	wantHit := now.Add(40 * time.Hour)
	if !f.HitAt.Equal(wantHit) {
		t.Fatalf("HitAt %v want %v", f.HitAt.UTC(), wantHit.UTC())
	}
	if !f.HitsBeforeWeekEnd {
		t.Fatal("expected hit before week end")
	}
}

func TestForecastWeeklyLimit_exceeded(t *testing.T) {
	t.Parallel()
	loc := time.UTC
	now := time.Date(2026, 3, 25, 12, 0, 0, 0, loc)
	answers := []model.AnswerReport{
		{Timestamp: now.Add(-1 * time.Hour).Format(time.RFC3339), CostUSD: 200},
	}
	f := ForecastWeeklyLimit(answers, now, 100, ForecastUSD)
	if !f.AlreadyExceeded {
		t.Fatal("want exceeded")
	}
	if f.HitAt != nil {
		t.Fatal("want no hit time")
	}
}

func TestForecastWeeklyLimit_noUsage(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	f := ForecastWeeklyLimit(nil, now, 100, ForecastUSD)
	if f.NoForecastReason == "" {
		t.Fatal("want reason")
	}
}
