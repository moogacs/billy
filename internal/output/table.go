package output

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/geekmonkey/billy/internal/model"
)

// PrintTable writes human-readable per-turn and per-answer tables to w.
func PrintTable(w io.Writer, rep model.AnalyzeReport, opts DisplayOptions) {
	lim := opts.PromptLimit()
	fmt.Fprintln(w, opts.Dim(rep.Meta.Disclaimer))
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n%s\n\n", opts.Dim("Session file"), opts.Blue("    "+rep.Meta.SessionPath))
	fmt.Fprintf(w, "  %s\n%s\n\n", opts.Dim("Vendor"), opts.Cyan("    "+string(rep.Meta.Vendor)))

	const width = 88
	opts.PrintSectionBanner(w, width, "By conversation turn (your question -> model usage for that turn)")

	tw := tabwriter.NewWriter(w, 0, 8, 2, ' ', 0)
	var turnCostSum float64
	if opts.ShowPrompts {
		hdr := "  #\tPrompt\tInput\tOutput\tCache read\tCache write\tEst. cost"
		if opts.Color {
			hdr = opts.Cyan(hdr)
		}
		fmt.Fprintln(tw, hdr)
		fmt.Fprintln(tw, "  -\t------\t-----\t------\t----------\t-----------\t---------")
		for _, t := range rep.Turns {
			prev := FormatPromptCell(t.Prompt, lim)
			fmt.Fprintf(tw, "  %d\t%s\t%s\t%s\t%s\t%s\t%s\n",
				t.Index, prev,
				formatIntHuman(t.Usage.InputTokens),
				formatIntHuman(t.Usage.OutputTokens),
				formatIntHuman(t.Usage.CacheReadInputTokens),
				formatIntHuman(t.Usage.CacheCreationInputTokens),
				opts.MoneyUSD(t.CostUSD),
			)
			turnCostSum += t.CostUSD
		}
	} else {
		hdr := "  #\tInput\tOutput\tCache read\tCache write\tEst. cost"
		if opts.Color {
			hdr = opts.Cyan(hdr)
		}
		fmt.Fprintln(tw, hdr)
		fmt.Fprintln(tw, "  -\t-----\t------\t----------\t-----------\t---------")
		for _, t := range rep.Turns {
			fmt.Fprintf(tw, "  %d\t%s\t%s\t%s\t%s\t%s\n",
				t.Index,
				formatIntHuman(t.Usage.InputTokens),
				formatIntHuman(t.Usage.OutputTokens),
				formatIntHuman(t.Usage.CacheReadInputTokens),
				formatIntHuman(t.Usage.CacheCreationInputTokens),
				opts.MoneyUSD(t.CostUSD),
			)
			turnCostSum += t.CostUSD
		}
	}
	_ = tw.Flush()

	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s %s\n", opts.Dim("Turn subtotal (estimated)"), opts.Bold(opts.MoneyUSD(turnCostSum)))
	fmt.Fprintln(w)

	opts.PrintSectionBanner(w, width, "Each API completion (same turn may have several calls)")

	turnPrompt := map[int]string{}
	if opts.ShowPrompts {
		for _, t := range rep.Turns {
			turnPrompt[t.Index] = FormatPromptCell(t.Prompt, lim)
		}
	}

	tw2 := tabwriter.NewWriter(w, 0, 8, 2, ' ', 0)
	var ansCostSum float64
	if opts.ShowPrompts {
		hdr := "  Turn\tTurn prompt\tModel\tInput\tOutput\tCache read\tCache write\tEst. cost\tPriced?"
		if opts.Color {
			hdr = opts.Cyan(hdr)
		}
		fmt.Fprintln(tw2, hdr)
		fmt.Fprintln(tw2, "  ----\t-----------\t-----\t-----\t------\t----------\t-----------\t---------\t-------")
		for _, a := range rep.Answers {
			priced := "yes"
			if a.UnknownModel {
				priced = "no (unknown model)"
				if opts.Color {
					priced = opts.Yellow(priced)
				}
			}
			tp := turnPrompt[a.TurnIndex]
			if tp == "" {
				tp = "—"
			}
			model := a.Model
			if opts.Color {
				model = opts.Magenta(model)
			}
			fmt.Fprintf(tw2, "  %d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				a.TurnIndex, tp, model,
				formatIntHuman(a.Usage.InputTokens),
				formatIntHuman(a.Usage.OutputTokens),
				formatIntHuman(a.Usage.CacheReadInputTokens),
				formatIntHuman(a.Usage.CacheCreationInputTokens),
				opts.MoneyUSD(a.CostUSD),
				priced,
			)
			ansCostSum += a.CostUSD
		}
	} else {
		hdr := "  Turn\tModel\tInput\tOutput\tCache read\tCache write\tEst. cost\tPriced?"
		if opts.Color {
			hdr = opts.Cyan(hdr)
		}
		fmt.Fprintln(tw2, hdr)
		fmt.Fprintln(tw2, "  ----\t-----\t-----\t------\t----------\t-----------\t---------\t-------")
		for _, a := range rep.Answers {
			priced := "yes"
			if a.UnknownModel {
				priced = "no (unknown model)"
				if opts.Color {
					priced = opts.Yellow(priced)
				}
			}
			model := a.Model
			if opts.Color {
				model = opts.Magenta(model)
			}
			fmt.Fprintf(tw2, "  %d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				a.TurnIndex, model,
				formatIntHuman(a.Usage.InputTokens),
				formatIntHuman(a.Usage.OutputTokens),
				formatIntHuman(a.Usage.CacheReadInputTokens),
				formatIntHuman(a.Usage.CacheCreationInputTokens),
				opts.MoneyUSD(a.CostUSD),
				priced,
			)
			ansCostSum += a.CostUSD
		}
	}
	_ = tw2.Flush()

	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s %s\n", opts.Dim("Completion subtotal (estimated)"), opts.Bold(opts.MoneyUSD(ansCostSum)))
	fmt.Fprintln(w)
	fmt.Fprintln(w, opts.Dim("  (Turn vs completion totals can differ slightly if pricing rounds per line.)"))
	if opts.ShowPrompts {
		fmt.Fprintln(w, opts.Dim(fmt.Sprintf("  (Prompt column width: %d runes; use --prompt-width to change; --show-prompts=false to hide.)", lim)))
	}
}
