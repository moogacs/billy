package output

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/geekmonkey/billy/internal/model"
)

func vendorTitle(v model.Vendor) string {
	switch v {
	case model.VendorAnthropic:
		return "Claude Code"
	case model.VendorOpenAI:
		return "Codex"
	case model.VendorCursor:
		return "Cursor"
	default:
		return string(v)
	}
}

// PrintAggregateByProvider prints each provider block then grand totals.
func PrintAggregateByProvider(w io.Writer, rep model.AggregateByProviderReport, opts DisplayOptions) {
	fmt.Fprintln(w, opts.Dim(rep.Meta.Disclaimer))
	fmt.Fprintln(w)
	for _, sec := range rep.Providers {
		printProviderSection(w, sec, opts)
	}
	fmt.Fprintln(w)
	printGrandHeader(w, opts)
	printKVTable(w, "  ", rep.GrandTotals, opts)
	if opts.ShowPrompts {
		fmt.Fprintf(w, "\n%s\n", opts.Dim(fmt.Sprintf("  (First prompt column: %d runes max; --show-prompts=false to hide.)", opts.PromptLimit())))
	}
}

func sectionHeader(w io.Writer, title string, opts DisplayOptions) {
	opts.PrintSectionBanner(w, 88, title)
}

func printGrandHeader(w io.Writer, opts DisplayOptions) {
	const width = 88
	line := strings.Repeat("=", width)
	fmt.Fprintln(w, opts.Dim(line))
	fmt.Fprintf(w, "  %s\n", opts.Yellow(opts.Bold("Grand total — all providers")))
	fmt.Fprintln(w, opts.Dim(line))
	fmt.Fprintln(w)
}

func printProviderSection(w io.Writer, sec model.ProviderAggregateSection, opts DisplayOptions) {
	if len(sec.Sessions) == 0 {
		return
	}
	title := vendorTitle(sec.Vendor)
	sectionHeader(w, fmt.Sprintf("%s (%s) · %d session files", title, sec.Vendor, len(sec.Sessions)), opts)

	lim := opts.PromptLimit()
	tw := tabwriter.NewWriter(w, 0, 10, 2, ' ', 0)
	if opts.ShowPrompts {
		hdr := "  Session file\tFirst prompt\tEst. cost\tInput\tOutput\tCache read\tCache write\tTurns\tCompletions\tUnpriced"
		if opts.Color {
			hdr = opts.Cyan(hdr)
		}
		fmt.Fprintln(tw, hdr)
		fmt.Fprintln(tw, "  ------------\t------------\t---------\t-----\t------\t----------\t-----------\t-----\t-----------\t--------")
		for _, row := range sec.Sessions {
			path := truncatePath(strings.ReplaceAll(row.SessionPath, "\t", " "), 44)
			if opts.Color {
				path = opts.Blue(path)
			}
			fp := FormatPromptCell(row.FirstPrompt, lim)
			if fp == "" {
				fp = "—"
			}
			unk := "—"
			if row.UnknownModelAnswers > 0 {
				unk = fmt.Sprintf("%d", row.UnknownModelAnswers)
				if opts.Color {
					unk = opts.Yellow(unk)
				}
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%d\t%s\n",
				path, fp,
				opts.MoneyUSD(row.CostUSD),
				formatIntHuman(row.Usage.InputTokens),
				formatIntHuman(row.Usage.OutputTokens),
				formatIntHuman(row.Usage.CacheReadInputTokens),
				formatIntHuman(row.Usage.CacheCreationInputTokens),
				row.Turns,
				row.Answers,
				unk,
			)
		}
	} else {
		hdr := "  Session file\tEst. cost\tInput\tOutput\tCache read\tCache write\tTurns\tCompletions\tUnpriced"
		if opts.Color {
			hdr = opts.Cyan(hdr)
		}
		fmt.Fprintln(tw, hdr)
		fmt.Fprintln(tw, "  ------------\t---------\t-----\t------\t----------\t-----------\t-----\t-----------\t--------")
		for _, row := range sec.Sessions {
			path := truncatePath(strings.ReplaceAll(row.SessionPath, "\t", " "), 50)
			if opts.Color {
				path = opts.Blue(path)
			}
			unk := "—"
			if row.UnknownModelAnswers > 0 {
				unk = fmt.Sprintf("%d", row.UnknownModelAnswers)
				if opts.Color {
					unk = opts.Yellow(unk)
				}
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%s\t%d\t%d\t%s\n",
				path,
				opts.MoneyUSD(row.CostUSD),
				formatIntHuman(row.Usage.InputTokens),
				formatIntHuman(row.Usage.OutputTokens),
				formatIntHuman(row.Usage.CacheReadInputTokens),
				formatIntHuman(row.Usage.CacheCreationInputTokens),
				row.Turns,
				row.Answers,
				unk,
			)
		}
	}
	_ = tw.Flush()

	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n", opts.Magenta(opts.Bold(fmt.Sprintf("Subtotal — %s", title))))
	st := sec.Totals
	printKVTable(w, "    ", st, opts)
	fmt.Fprintln(w)
}

func printKVTable(w io.Writer, indent string, t model.AggregateTotalsRow, opts DisplayOptions) {
	if opts.Color {
		kv := func(k, v string) {
			lbl := fmt.Sprintf("%-26s", k)
			fmt.Fprintf(w, "%s%s %s\n", indent, opts.Dim(lbl), v)
		}
		kv("Estimated cost", opts.MoneyUSD(t.CostUSD))
		kv("Input tokens", formatIntHuman(t.Usage.InputTokens))
		kv("Output tokens", formatIntHuman(t.Usage.OutputTokens))
		kv("Cache read tokens", formatIntHuman(t.Usage.CacheReadInputTokens))
		kv("Cache write tokens", formatIntHuman(t.Usage.CacheCreationInputTokens))
		kv("Conversation turns", formatIntHuman(t.Turns))
		kv("API completions", formatIntHuman(t.Answers))
		if t.UnknownModelAnswers > 0 {
			kv("Unpriced (unknown model)", opts.Yellow(fmt.Sprintf("%d", t.UnknownModelAnswers)))
		}
		return
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "%sEstimated cost\t%s\n", indent, formatUSD(t.CostUSD))
	fmt.Fprintf(tw, "%sInput tokens\t%s\n", indent, formatIntHuman(t.Usage.InputTokens))
	fmt.Fprintf(tw, "%sOutput tokens\t%s\n", indent, formatIntHuman(t.Usage.OutputTokens))
	fmt.Fprintf(tw, "%sCache read tokens\t%s\n", indent, formatIntHuman(t.Usage.CacheReadInputTokens))
	fmt.Fprintf(tw, "%sCache write tokens\t%s\n", indent, formatIntHuman(t.Usage.CacheCreationInputTokens))
	fmt.Fprintf(tw, "%sConversation turns\t%s\n", indent, formatIntHuman(t.Turns))
	fmt.Fprintf(tw, "%sAPI completions\t%s\n", indent, formatIntHuman(t.Answers))
	if t.UnknownModelAnswers > 0 {
		fmt.Fprintf(tw, "%sUnpriced (unknown model)\t%d\n", indent, t.UnknownModelAnswers)
	}
	_ = tw.Flush()
}

// SortAggregateSessions sorts rows by path for stable output.
func SortAggregateSessions(rows []model.AggregateSessionRow) {
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].SessionPath < rows[j].SessionPath
	})
}
