package analyze

import (
	"strings"
	"time"

	"github.com/geekmonkey/billy/internal/model"
)

// ParseLogTime parses timestamps from local session logs. Layouts are tried in order.
func ParseLogTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// DayBounds returns the half-open interval [start, end) for the calendar day of now
// in now's location.
func DayBounds(now time.Time) (start, end time.Time) {
	loc := now.Location()
	start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	return start, start.Add(24 * time.Hour)
}

// MonthBounds returns the half-open interval [start, end) for the calendar month of now
// in now's location.
func MonthBounds(now time.Time) (start, end time.Time) {
	loc := now.Location()
	start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	return start, start.AddDate(0, 1, 0)
}

// WeekBounds returns the half-open interval [start, end) for the ISO-style calendar week
// of now in now's location: Monday 00:00 through the following Monday 00:00.
func WeekBounds(now time.Time) (start, end time.Time) {
	loc := now.Location()
	daysSinceMonday := (int(now.Weekday()) + 6) % 7 // Monday=0 .. Sunday=6
	start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -daysSinceMonday)
	return start, start.AddDate(0, 0, 7)
}

// answerInWindow reports whether a completion timestamp falls in [start, end).
// Answers with missing or unparseable timestamps are excluded.
func answerInWindow(a model.AnswerReport, start, end time.Time) bool {
	tm, ok := ParseLogTime(a.Timestamp)
	if !ok {
		return false
	}
	return !tm.Before(start) && tm.Before(end)
}

// FilterReportByWindow keeps only assistant completions whose timestamps fall in [start, end).
// Turns with no matching completions are dropped; turn indices are renumbered.
func FilterReportByWindow(rep model.AnalyzeReport, start, end time.Time) model.AnalyzeReport {
	var turns []model.TurnReport
	var flat []model.AnswerReport
	newIdx := 0

	for _, t := range rep.Turns {
		var kept []model.AnswerReport
		for _, a := range t.Answers {
			if answerInWindow(a, start, end) {
				kept = append(kept, a)
			}
		}
		if len(kept) == 0 {
			continue
		}

		var turnUsage model.UsageBreakdown
		var turnCost float64
		for i := range kept {
			kept[i].TurnIndex = newIdx
			turnUsage = turnUsage.Add(kept[i].Usage)
			turnCost += kept[i].CostUSD
		}

		turns = append(turns, model.TurnReport{
			Index:           newIdx,
			Prompt:          t.Prompt,
			PromptTimestamp: t.PromptTimestamp,
			Usage:           turnUsage,
			CostUSD:         turnCost,
			Answers:         kept,
		})
		for _, a := range kept {
			flat = append(flat, a)
		}
		newIdx++
	}

	rep.Turns = turns
	rep.Answers = flat
	return rep
}
