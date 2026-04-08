package analyze

import (
	"math"
	"time"

	"github.com/geekmonkey/billy/internal/model"
)

// ForecastMetric selects which scalar from priced answers drives the limit.
type ForecastMetric int

const (
	ForecastUSD ForecastMetric = iota
	ForecastTokens
)

// WeeklyForecast is a projection from average hourly pace so far this week.
type WeeklyForecast struct {
	WeekStart time.Time
	WeekEnd   time.Time
	Now       time.Time
	Metric    ForecastMetric

	Used         float64
	Limit        float64
	Remaining    float64
	ElapsedHours float64
	HourlyRate   float64

	// HitAt is set when a finite projection exists (pace > 0 and remaining > 0).
	HitAt *time.Time
	// HitsBeforeWeekEnd is true when HitAt is before the next week boundary.
	HitsBeforeWeekEnd bool
	AlreadyExceeded   bool
	// NoForecastReason is set when HitAt is nil (e.g. no usage yet, or no timestamps).
	NoForecastReason string
}

const minElapsedHours = 1.0

// SumTotalTokens returns input+output+cache fields summed (one “token” bill shape).
func SumTotalTokens(u model.UsageBreakdown) int64 {
	return int64(u.InputTokens + u.OutputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens)
}

func metricValue(a model.AnswerReport, m ForecastMetric) float64 {
	switch m {
	case ForecastUSD:
		return a.CostUSD
	case ForecastTokens:
		return float64(SumTotalTokens(a.Usage))
	default:
		return 0
	}
}

// completionThisWeekSoFar reports whether the completion timestamp is in
// [weekStart, now] (local inclusive bounds on the end for “so far”).
func completionThisWeekSoFar(a model.AnswerReport, weekStart, now time.Time) bool {
	tm, ok := ParseLogTime(a.Timestamp)
	if !ok {
		return false
	}
	return !tm.Before(weekStart) && !tm.After(now)
}

// SumAnswersInWeek sums the chosen metric for answers in [weekStart, now] with parseable times.
func SumAnswersInWeek(answers []model.AnswerReport, weekStart, now time.Time, m ForecastMetric) (sum float64, counted int) {
	for i := range answers {
		if !completionThisWeekSoFar(answers[i], weekStart, now) {
			continue
		}
		sum += metricValue(answers[i], m)
		counted++
	}
	return sum, counted
}

// ForecastWeeklyLimit projects when a weekly cap is reached using average hourly
// pace since week start (minimum denominator minElapsedHours). Completions
// without parseable timestamps are ignored for both usage and pacing.
func ForecastWeeklyLimit(answers []model.AnswerReport, now time.Time, limit float64, m ForecastMetric) WeeklyForecast {
	weekStart, weekEnd := WeekBounds(now)
	out := WeeklyForecast{
		WeekStart: weekStart,
		WeekEnd:   weekEnd,
		Now:       now,
		Metric:    m,
		Limit:     limit,
	}

	if limit <= 0 {
		out.NoForecastReason = "limit must be positive"
		return out
	}

	used, _ := SumAnswersInWeek(answers, weekStart, now, m)
	out.Used = used
	out.Remaining = limit - used

	if used >= limit {
		out.AlreadyExceeded = true
		return out
	}

	elapsed := now.Sub(weekStart).Hours()
	if elapsed <= 0 {
		out.NoForecastReason = "now is not after week start"
		return out
	}
	denom := math.Max(elapsed, minElapsedHours)
	out.ElapsedHours = elapsed
	out.HourlyRate = used / denom

	if used <= 0 || out.HourlyRate <= 0 {
		out.NoForecastReason = "no usage with parseable timestamps this week yet; cannot estimate hourly pace"
		return out
	}

	rem := limit - used
	hoursToHit := rem / out.HourlyRate
	hit := now.Add(time.Duration(hoursToHit * float64(time.Hour)))
	out.HitAt = &hit
	out.HitsBeforeWeekEnd = hit.Before(weekEnd)
	return out
}

// WeeklyForecastReport is stable JSON for forecast runs.
type WeeklyForecastReport struct {
	WeekStart string `json:"week_start"`
	WeekEnd   string `json:"week_end"`
	Now       string `json:"now"`
	Metric    string `json:"metric"`

	Used         float64 `json:"used"`
	Limit        float64 `json:"limit"`
	Remaining    float64 `json:"remaining"`
	ElapsedHours float64 `json:"elapsed_hours"`
	HourlyRate   float64 `json:"hourly_rate"`

	HitAt             *string `json:"hit_at,omitempty"`
	HitsBeforeWeekEnd bool    `json:"hits_before_week_end"`
	AlreadyExceeded   bool    `json:"already_exceeded"`
	NoForecastReason  string  `json:"no_forecast_reason,omitempty"`
}

// ReportRFC3339 formats f for JSON with timestamps in loc.
func (f WeeklyForecast) ReportRFC3339(loc *time.Location) WeeklyForecastReport {
	metric := "usd"
	if f.Metric == ForecastTokens {
		metric = "tokens"
	}
	r := WeeklyForecastReport{
		WeekStart: f.WeekStart.In(loc).Format(time.RFC3339),
		WeekEnd:   f.WeekEnd.In(loc).Format(time.RFC3339),
		Now:       f.Now.In(loc).Format(time.RFC3339),
		Metric:    metric,

		Used:         f.Used,
		Limit:        f.Limit,
		Remaining:    f.Remaining,
		ElapsedHours: f.ElapsedHours,
		HourlyRate:   f.HourlyRate,

		HitsBeforeWeekEnd: f.HitsBeforeWeekEnd,
		AlreadyExceeded:   f.AlreadyExceeded,
		NoForecastReason:  f.NoForecastReason,
	}
	if f.HitAt != nil {
		s := f.HitAt.In(loc).Format(time.RFC3339)
		r.HitAt = &s
	}
	return r
}
