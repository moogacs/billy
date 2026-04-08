package analyze

import (
	"sort"
	"time"

	"github.com/geekmonkey/billy/internal/model"
)

// DayBucket holds aggregated usage for a single calendar day.
type DayBucket struct {
	Date    time.Time
	CostUSD float64
	Tokens  int64
}

// BucketByDay aggregates answers into per-day buckets within [from, to].
// Answers with unparseable or missing timestamps are silently skipped.
func BucketByDay(answers []model.AnswerReport, from, to time.Time) []DayBucket {
	m := map[time.Time]*DayBucket{}
	for _, a := range answers {
		t, ok := ParseLogTime(a.Timestamp)
		if !ok {
			continue
		}
		t = t.In(time.Local)
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
		if day.Before(from) || day.After(to) {
			continue
		}
		b, ok := m[day]
		if !ok {
			b = &DayBucket{Date: day}
			m[day] = b
		}
		b.CostUSD += a.CostUSD
		b.Tokens += int64(a.Usage.InputTokens + a.Usage.OutputTokens +
			a.Usage.CacheReadInputTokens + a.Usage.CacheCreationInputTokens)
	}
	out := make([]DayBucket, 0, len(m))
	for _, b := range m {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Date.Before(out[j].Date)
	})
	return out
}
