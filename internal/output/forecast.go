package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/geekmonkey/billy/internal/analyze"
)

// PrintForecast writes a human-readable weekly cap projection.
func PrintForecast(w io.Writer, f analyze.WeeklyForecast, opts DisplayOptions) {
	loc := f.Now.Location()
	fmt.Fprintln(w, opts.Dim("Estimate from average hourly pace since local week start (Monday 00:00). Not a guarantee; logs may omit timestamps."))
	fmt.Fprintln(w)

	metricLabel := "Estimated USD"
	if f.Metric == analyze.ForecastTokens {
		metricLabel = "Tokens (input+output+cache)"
	}

	fmt.Fprintf(w, "  %s\n%s\n\n", opts.Dim("Week (local)"), opts.Cyan(fmt.Sprintf("    %s → %s",
		f.WeekStart.In(loc).Format("Mon Jan _2 15:04"),
		f.WeekEnd.In(loc).Format("Mon Jan _2 15:04"))))
	fmt.Fprintf(w, "  %s\n%s\n\n", opts.Dim("Metric"), opts.Cyan("    "+metricLabel))

	fmt.Fprintf(w, "  %s\n", opts.Dim("Pace & cap"))
	tw := fmt.Sprintf("    Used %s / limit %s  ·  remaining %s\n",
		formatForecastScalar(f.Used, f.Metric),
		formatForecastScalar(f.Limit, f.Metric),
		formatForecastScalar(f.Remaining, f.Metric))
	fmt.Fprintf(w, "%s", opts.Cyan(tw))
	fmt.Fprintf(w, "    Elapsed this week: %.1f h  ·  avg pace: %s/h\n\n",
		f.ElapsedHours,
		opts.Bold(formatForecastScalar(f.HourlyRate, f.Metric)))

	if f.AlreadyExceeded {
		fmt.Fprintf(w, "  %s\n", opts.Bold(opts.Yellow("Already at or over the weekly cap in these logs.")))
		return
	}
	if f.NoForecastReason != "" {
		fmt.Fprintf(w, "  %s %s\n", opts.Dim("No projection:"), f.NoForecastReason)
		return
	}
	if f.HitAt == nil {
		return
	}

	hitLocal := f.HitAt.In(loc)
	line := fmt.Sprintf("At this pace, reach the cap around %s (%s).",
		hitLocal.Format("Mon Jan _2 15:04"), loc.String())
	if !f.HitsBeforeWeekEnd {
		line = fmt.Sprintf("At this pace, you are unlikely to hit the cap before the week resets %s (%s).",
			f.WeekEnd.In(loc).Format("Mon Jan _2 15:04"), loc.String())
	}
	fmt.Fprintf(w, "  %s\n", opts.Bold(line))
}

func formatForecastScalar(v float64, m analyze.ForecastMetric) string {
	if m == analyze.ForecastTokens {
		return fmt.Sprintf("%.0f", v)
	}
	s := fmt.Sprintf("%.2f", v)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" {
		return "0"
	}
	return s
}
