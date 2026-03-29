package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/geekmonkey/billbot/internal/analyze"
	"github.com/geekmonkey/billbot/internal/anthropic"
	"github.com/geekmonkey/billbot/internal/codex"
	"github.com/geekmonkey/billbot/internal/cursor"
	"github.com/geekmonkey/billbot/internal/model"
	"github.com/geekmonkey/billbot/internal/output"
	"github.com/geekmonkey/billbot/internal/pricing"
	"github.com/geekmonkey/billbot/internal/provider"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var (
	flagPricing      string
	flagFormat       string
	flagProvider     string
	flagAgents       bool
	flagLatestOnly   bool
	flagShowPrompts  bool
	flagPromptWidth  int
	flagColor        string
	flagNoColor      bool
	flagDaily        bool
	flagMonthly      bool
)

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "billbot [path]",
		Short: "Estimate Claude Code, Codex & Cursor API cost from local session logs",
		Long: `Privacy-first CLI: estimate spend from local Claude Code (Anthropic), OpenAI Codex, and Cursor session files—no network calls. Figures come from embedded or custom pricing YAML; they are estimates, not invoices.

For a directory, the default is to scan every session file, grouped by provider (Claude / Codex / Cursor). Use --latest-only to pick a single newest log instead. --provider limits which families are included; auto includes all three.

Use --daily or --monthly to count only API completions whose timestamps fall in the current local calendar day or month (completions without a parseable time are omitted).`,
		Args: cobra.MaximumNArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if flagDaily && flagMonthly {
				return fmt.Errorf("use only one of --daily and --monthly")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			p := "."
			if len(args) > 0 {
				p = args[0]
			}
			return runAnalyzePath(p)
		},
	}

	root.PersistentFlags().StringVar(&flagPricing, "pricing-file", "", "path to pricing YAML (default: embedded)")
	root.PersistentFlags().StringVar(&flagFormat, "format", "table", "output format: table or json")
	root.PersistentFlags().StringVar(&flagProvider, "provider", "auto", "auto | anthropic|cc | openai|codex | cursor")
	root.PersistentFlags().BoolVar(&flagAgents, "include-agents", false, "when resolving Claude paths, include agent-*.jsonl")
	root.PersistentFlags().BoolVar(&flagLatestOnly, "latest-only", false, "with a directory: analyze only the single newest session (legacy behavior)")
	root.PersistentFlags().BoolVar(&flagShowPrompts, "show-prompts", false, "include prompt text in tables (turns, completions, aggregate first prompt)")
	root.PersistentFlags().IntVar(&flagPromptWidth, "prompt-width", 72, "max runes per prompt cell (8-32000)")
	root.PersistentFlags().StringVar(&flagColor, "color", "auto", "auto|always|never: ANSI colors for table output (respects NO_COLOR, FORCE_COLOR)")
	root.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "disable colors (same as --color=never)")
	root.PersistentFlags().BoolVar(&flagDaily, "daily", false, "only completions from today (local calendar day)")
	root.PersistentFlags().BoolVar(&flagMonthly, "monthly", false, "only completions from the current calendar month (local timezone)")

	analyzeCmd := &cobra.Command{
		Use:   "analyze [session.jsonl|cursor.db|project-dir]",
		Short: "Analyze a file or aggregate sessions from default agent data dirs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := "."
			if len(args) > 0 {
				p = args[0]
			}
			return runAnalyzePath(p)
		},
	}

	projectCmd := &cobra.Command{
		Use:   "project <project-dir>",
		Short: "Same as analyze <dir> (aggregate by provider unless --latest-only)",
		Args:  cobra.ExactArgs(1),
		RunE:  runProject,
	}

	root.AddCommand(analyzeCmd, projectCmd)
	return root
}

func runAnalyzePath(path string) error {
	pt, err := pricing.Load(flagPricing)
	if err != nil {
		return err
	}
	pn, err := provider.ParseProviderFlag(flagProvider)
	if err != nil {
		return err
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return err
	}

	if st.IsDir() && !flagLatestOnly {
		active, start, end := timeFilterFromFlags()
		return runDefaultDirectoryAggregate(pn, pt, displayOptions(), active, start, end)
	}

	sessionPath := abs
	if st.IsDir() {
		var v model.Vendor
		var usedFallback bool
		sessionPath, v, usedFallback, err = provider.ResolveDirToSession(pn, abs, flagAgents)
		if err != nil {
			return err
		}
		if usedFallback && (pn == provider.Auto || pn == provider.Anthropic) {
			if pn == provider.Anthropic && v == model.VendorAnthropic {
				fmt.Fprintf(os.Stderr, "billbot: no session for project %s; using latest Claude Code log: %s\n", abs, sessionPath)
			} else if pn == provider.Auto {
				fmt.Fprintf(os.Stderr, "billbot: no Claude session for project %s; using latest %s log: %s\n", abs, provider.VendorLabel(v), sessionPath)
			}
		}
		pn = provider.ProviderName(v)
	}

	vendor, events, err := provider.ReadSession(pn, sessionPath)
	if err != nil {
		return err
	}
	rep := analyze.BuildReport(sessionPath, vendor, events, pt)
	if active, start, end := timeFilterFromFlags(); active {
		rep = analyze.FilterReportByWindow(rep, start, end)
	}
	return emit(rep, displayOptions())
}

func runProject(cmd *cobra.Command, args []string) error {
	return runAnalyzePath(args[0])
}

const (
	minPromptWidth = 8
	maxPromptWidth = 32000
)

func displayOptions() output.DisplayOptions {
	w := flagPromptWidth
	if w < minPromptWidth {
		w = minPromptWidth
	}
	if w > maxPromptWidth {
		w = maxPromptWidth
	}
	return output.DisplayOptions{
		ShowPrompts:    flagShowPrompts,
		PromptMaxRunes: w,
		Color:          colorEnabled(),
	}
}

func colorEnabled() bool {
	if flagNoColor {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(flagColor)) {
	case "never", "false", "0", "off":
		return false
	case "always", "true", "1", "on":
		return true
	}
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" || os.Getenv("CLICOLOR_FORCE") != "" {
		return true
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func cursorReadSQLite(p string) ([]model.NormalizedEvent, error) {
	return cursor.ReadEventsFromSQLite(p)
}

func timeFilterFromFlags() (active bool, start, end time.Time) {
	if flagDaily {
		s, e := analyze.DayBounds(time.Now())
		return true, s, e
	}
	if flagMonthly {
		s, e := analyze.MonthBounds(time.Now())
		return true, s, e
	}
	return false, time.Time{}, time.Time{}
}

func buildProviderSection(paths []string, v model.Vendor, read func(string) ([]model.NormalizedEvent, error), pt *pricing.Table, opts output.DisplayOptions, filterTime bool, winStart, winEnd time.Time) model.ProviderAggregateSection {
	var sec model.ProviderAggregateSection
	sec.Vendor = v
	if len(paths) == 0 {
		return sec
	}
	for _, p := range paths {
		ev, err := read(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "billbot: skip %s: %v\n", p, err)
			continue
		}
		rep := analyze.BuildReport(p, v, ev, pt)
		if filterTime {
			rep = analyze.FilterReportByWindow(rep, winStart, winEnd)
		}
		c, u, ans, unk := analyze.SumAnswers(rep)
		if filterTime && ans == 0 {
			continue
		}
		first := ""
		if len(rep.Turns) > 0 {
			first = rep.Turns[0].Prompt
		}
		sec.Sessions = append(sec.Sessions, model.AggregateSessionRow{
			SessionPath:         p,
			FirstPrompt:         first,
			CostUSD:             c,
			Usage:               u,
			Turns:               len(rep.Turns),
			Answers:             ans,
			UnknownModelAnswers: unk,
		})
		sec.Totals.CostUSD += c
		sec.Totals.Usage = sec.Totals.Usage.Add(u)
		sec.Totals.Turns += len(rep.Turns)
		sec.Totals.Answers += ans
		sec.Totals.UnknownModelAnswers += unk
	}
	output.SortAggregateSessions(sec.Sessions)
	sec.SessionFiles = len(sec.Sessions)
	return sec
}

func appendNonEmpty(out []model.ProviderAggregateSection, sec model.ProviderAggregateSection) []model.ProviderAggregateSection {
	if len(sec.Sessions) == 0 {
		return out
	}
	return append(out, sec)
}

func runDefaultDirectoryAggregate(pn provider.Name, pt *pricing.Table, opts output.DisplayOptions, filterTime bool, winStart, winEnd time.Time) error {
	var sections []model.ProviderAggregateSection
	switch pn {
	case provider.Auto:
		sections = appendNonEmpty(sections, buildProviderSection(
			anthropic.AllAnthropicSessionPaths(flagAgents), model.VendorAnthropic, anthropic.ReadEvents, pt, opts, filterTime, winStart, winEnd))
		sections = appendNonEmpty(sections, buildProviderSection(
			codex.CollectSessionJSONLPaths(), model.VendorOpenAI, codex.ReadEvents, pt, opts, filterTime, winStart, winEnd))
		sections = appendNonEmpty(sections, buildProviderSection(
			cursor.CollectWorkspaceStateDBs(), model.VendorCursor, cursorReadSQLite, pt, opts, filterTime, winStart, winEnd))
	case provider.Anthropic:
		sections = appendNonEmpty(sections, buildProviderSection(
			anthropic.AllAnthropicSessionPaths(flagAgents), model.VendorAnthropic, anthropic.ReadEvents, pt, opts, filterTime, winStart, winEnd))
	case provider.OpenAI:
		sections = appendNonEmpty(sections, buildProviderSection(
			codex.CollectSessionJSONLPaths(), model.VendorOpenAI, codex.ReadEvents, pt, opts, filterTime, winStart, winEnd))
	case provider.CursorProv:
		sections = appendNonEmpty(sections, buildProviderSection(
			cursor.CollectWorkspaceStateDBs(), model.VendorCursor, cursorReadSQLite, pt, opts, filterTime, winStart, winEnd))
	default:
		return fmt.Errorf("unknown provider %q", pn)
	}
	if len(sections) == 0 {
		if filterTime {
			return fmt.Errorf("no usage in the selected time window for provider %q (try without --daily/--monthly, or check timestamps in your logs)", pn)
		}
		return fmt.Errorf("no session files found for provider %q (try --latest-only or a different --provider)", pn)
	}
	var grand model.AggregateTotalsRow
	for _, s := range sections {
		grand.CostUSD += s.Totals.CostUSD
		grand.Usage = grand.Usage.Add(s.Totals.Usage)
		grand.Turns += s.Totals.Turns
		grand.Answers += s.Totals.Answers
		grand.UnknownModelAnswers += s.Totals.UnknownModelAnswers
	}
	var rep model.AggregateByProviderReport
	rep.Meta.Disclaimer = pt.DisclaimerLine()
	rep.Providers = sections
	rep.GrandTotals = grand
	return emitAggregateByProvider(rep, displayOptions())
}

func emitAggregateByProvider(rep model.AggregateByProviderReport, opts output.DisplayOptions) error {
	switch flagFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	case "table":
		output.PrintAggregateByProvider(os.Stdout, rep, opts)
		return nil
	default:
		return fmt.Errorf("unknown format %q", flagFormat)
	}
}

func emit(rep model.AnalyzeReport, opts output.DisplayOptions) error {
	switch flagFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	case "table":
		output.PrintTable(os.Stdout, rep, opts)
		return nil
	default:
		return fmt.Errorf("unknown format %q", flagFormat)
	}
}
