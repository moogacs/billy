package output

import (
	"fmt"
	"io"
	"time"

	"github.com/geekmonkey/billy/internal/analyze"
)

// HeatmapOptions controls what the heatmap measures and how many weeks it spans.
type HeatmapOptions struct {
	Metric string // "cost" or "tokens"
	Weeks  int    // number of weeks to display (default 52)
}

const (
	HeatmapMetricCost   = "cost"
	HeatmapMetricTokens = "tokens"
)

var heatmapChars = [5]string{"··", "░░", "▒▒", "▓▓", "██"}

func intensityLevel(v, maxVal float64) int {
	if v <= 0 || maxVal <= 0 {
		return 0
	}
	frac := v / maxVal
	switch {
	case frac <= 0.25:
		return 1
	case frac <= 0.50:
		return 2
	case frac <= 0.75:
		return 3
	default:
		return 4
	}
}

func (o DisplayOptions) heatmapCell(level int) string {
	c := heatmapChars[level]
	switch level {
	case 0:
		return o.Dim(c)
	case 1:
		return o.wrap("2;32", c) // dark green
	case 2:
		return o.Green(c) // green
	case 3:
		return o.wrap("1;32", c) // bold green
	default:
		return o.wrap("1;92", c) // bold bright green
	}
}

// PrintHeatmap renders a GitHub-style calendar heatmap to w.
// Columns are weeks (oldest left, newest right); rows are Mon–Sun.
func PrintHeatmap(w io.Writer, buckets []analyze.DayBucket, opts DisplayOptions, hm HeatmapOptions) {
	if hm.Weeks <= 0 {
		hm.Weeks = 52
	}
	if hm.Metric == "" {
		hm.Metric = HeatmapMetricCost
	}

	now := time.Now().In(time.Local)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	// Align grid start to the Monday of the week that is (Weeks-1) weeks ago.
	daysSinceMonday := (int(today.Weekday()) + 6) % 7 // Monday=0..Sunday=6
	currentMonday := today.AddDate(0, 0, -daysSinceMonday)
	startMonday := currentMonday.AddDate(0, 0, -(hm.Weeks-1)*7)

	// Build O(1) value lookup keyed by day-truncated date.
	type cell struct {
		costUSD float64
		tokens  int64
	}
	dayMap := map[time.Time]cell{}
	for _, b := range buckets {
		dayMap[b.Date] = cell{costUSD: b.CostUSD, tokens: b.Tokens}
	}

	cellVal := func(d time.Time) float64 {
		c, ok := dayMap[d]
		if !ok {
			return 0
		}
		if hm.Metric == HeatmapMetricTokens {
			return float64(c.tokens)
		}
		return c.costUSD
	}

	// Find the max value across the window for relative intensity.
	var maxVal float64
	for d := startMonday; !d.After(today); d = d.AddDate(0, 0, 1) {
		if v := cellVal(d); v > maxVal {
			maxVal = v
		}
	}

	weeks := hm.Weeks

	// -- Month label row --------------------------------------------------
	// Each week column is 3 chars wide ("X  "). Day-label prefix is 7 chars.
	// We write the 3-char month abbreviation starting at the column where
	// each new month begins; labels are wide enough that they read naturally.
	const (
		dayLabelWidth = 7 // "  Mon  "
		cellWidth     = 3 // block char + 2 spaces
	)
	labelBuf := make([]byte, dayLabelWidth+weeks*cellWidth+4)
	for i := range labelBuf {
		labelBuf[i] = ' '
	}
	lastMonth := time.Month(0)
	for wi := 0; wi < weeks; wi++ {
		d := startMonday.AddDate(0, 0, wi*7)
		if d.Month() != lastMonth {
			pos := dayLabelWidth + wi*cellWidth
			name := d.Month().String()[:3]
			if pos+len(name) <= len(labelBuf) {
				copy(labelBuf[pos:], name)
			}
			lastMonth = d.Month()
		}
	}
	// Trim trailing spaces and print dimmed.
	end := len(labelBuf)
	for end > 0 && labelBuf[end-1] == ' ' {
		end--
	}
	fmt.Fprintln(w, opts.Dim(string(labelBuf[:end])))

	// -- Grid rows (Mon–Sun) ----------------------------------------------
	dayNames := [7]string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	for di := 0; di < 7; di++ {
		fmt.Fprintf(w, "  %s  ", dayNames[di])
		for wi := 0; wi < weeks; wi++ {
			d := startMonday.AddDate(0, 0, wi*7+di)
			if d.After(today) {
				// Future day in the current week: blank.
				fmt.Fprintf(w, "   ")
				continue
			}
			level := intensityLevel(cellVal(d), maxVal)
			fmt.Fprintf(w, "%s ", opts.heatmapCell(level))
		}
		fmt.Fprintf(w, "\n\n")
	}

	// -- Legend + summary -------------------------------------------------
	fmt.Fprintln(w)
	legend := fmt.Sprintf("  Less  %s %s %s %s %s  More",
		opts.heatmapCell(0), opts.heatmapCell(1), opts.heatmapCell(2),
		opts.heatmapCell(3), opts.heatmapCell(4))

	var totalVal float64
	for _, b := range buckets {
		if hm.Metric == HeatmapMetricTokens {
			totalVal += float64(b.Tokens)
		} else {
			totalVal += b.CostUSD
		}
	}
	var totalStr string
	if hm.Metric == HeatmapMetricTokens {
		totalStr = fmt.Sprintf("total: %s tokens", formatIntHuman(int(totalVal)))
	} else {
		totalStr = fmt.Sprintf("total: %s", formatUSD(totalVal))
	}

	fmt.Fprintf(w, "%s    %s\n",
		legend,
		opts.Dim(fmt.Sprintf("metric: %s · weeks: %d · %s", hm.Metric, weeks, totalStr)))
}
