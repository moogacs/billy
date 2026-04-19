package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/geekmonkey/billy/internal/analyze"
	"github.com/geekmonkey/billy/internal/anthropic"
	"github.com/geekmonkey/billy/internal/codex"
	"github.com/geekmonkey/billy/internal/cursor"
	"github.com/geekmonkey/billy/internal/model"
	"github.com/geekmonkey/billy/internal/output"
	"github.com/geekmonkey/billy/internal/pricing"
	"github.com/geekmonkey/billy/internal/provider"
	"github.com/geekmonkey/billy/internal/proxy"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Set by GoReleaser via -ldflags (-X main.*).
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func buildVersion() string {
	if commit == "" {
		return version
	}
	short := commit
	if len(short) > 7 {
		short = short[:7]
	}
	s := version + " (commit " + short + ")"
	if date != "" {
		s += ", built " + date
	}
	return s
}

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var (
	flagPricing           string
	flagFormat            string
	flagProvider          string
	flagAgents            bool
	flagLatestOnly        bool
	flagShowPrompts       bool
	flagPromptWidth       int
	flagColor             string
	flagNoColor           bool
	flagDaily             bool
	flagMonthly           bool
	flagWeeklyLimitUSD    float64
	flagWeeklyLimitTokens int64
	flagHeatmapMetric     string
	flagHeatmapWeeks      int
	flagHeatmapMonthly    bool
	flagInitAgent         string
	flagInitUninstall     bool
	flagInitProject       bool
)

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "billy [path]",
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

	forecastCmd := &cobra.Command{
		Use:   "forecast [path]",
		Short: "Project when a weekly USD or token cap is reached (hourly pace)",
		Long: `Uses the same log discovery as analyze: optional path to a session file or project dir, otherwise all default sessions for the selected provider(s).

The week is the local calendar week Monday 00:00 → next Monday 00:00. Pace is total usage since week start divided by elapsed hours (minimum 1h). Set exactly one of --weekly-limit-usd or --weekly-limit-tokens.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := "."
			if len(args) > 0 {
				p = args[0]
			}
			return runForecast(p)
		},
	}
	forecastCmd.Flags().Float64Var(&flagWeeklyLimitUSD, "weekly-limit-usd", 0, "weekly spend cap in USD (priced like analyze)")
	forecastCmd.Flags().Int64Var(&flagWeeklyLimitTokens, "weekly-limit-tokens", 0, "weekly token cap (sum of input+output+cache fields)")

	heatmapCmd := &cobra.Command{
		Use:   "heatmap [path]",
		Short: "Show a calendar heatmap of token burn or cost (GitHub contribution-style)",
		Long: `Renders a 7-row × N-week calendar grid where each cell represents one day, colored by spend or token usage intensity.

Uses the same session discovery as analyze: optional path to a file or directory, otherwise all default sessions for the selected provider(s). Days with no parseable timestamps are shown as empty.`,
		Args: cobra.MaximumNArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if flagHeatmapWeeks > 0 && flagHeatmapMonthly {
				return fmt.Errorf("use only one of --weeks and --monthly")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			p := "."
			if len(args) > 0 {
				p = args[0]
			}
			return runHeatmap(p)
		},
	}
	heatmapCmd.Flags().StringVar(&flagHeatmapMetric, "metric", "cost", "what to measure: cost or tokens")
	heatmapCmd.Flags().IntVar(&flagHeatmapWeeks, "weeks", 52, "number of weeks to display (1–260)")
	heatmapCmd.Flags().BoolVar(&flagHeatmapMonthly, "monthly", false, "show last 4 weeks only (same as --weeks 4)")

	proxyCmd := &cobra.Command{
		Use:   "proxy <command> [args...]",
		Short: "Run command and print compact output to save tokens",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runProxy,
	}
	gainCmd := &cobra.Command{
		Use:   "gain",
		Short: "Show estimated token savings from billy proxy",
		RunE:  runGain,
	}
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Install or remove billy runtime integration (global by default)",
		RunE:  runInit,
	}
	initCmd.Flags().StringVar(&flagInitAgent, "agent", "all", "target agent: all | cursor | codex | claude")
	initCmd.Flags().BoolVar(&flagInitUninstall, "uninstall", false, "remove previously installed billy integration")
	initCmd.Flags().BoolVar(&flagInitProject, "project", false, "install in current repo instead of global user config")

	proxyHookCmd := &cobra.Command{
		Use:    "proxy-hook",
		Short:  "Internal hook entrypoint for shell rewrite",
		Hidden: true,
		RunE:   runProxyHook,
	}

	root.AddCommand(analyzeCmd, projectCmd, forecastCmd, heatmapCmd, proxyCmd, gainCmd, initCmd, proxyHookCmd)
	root.Version = buildVersion()
	return root
}

func runProxy(cmd *cobra.Command, args []string) error {
	res, err := proxy.Run(args)
	if strings.TrimSpace(res.Output) != "" {
		fmt.Fprintln(os.Stdout, res.Output)
	}
	if err == nil {
		return nil
	}
	var exitErr proxy.CommandExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("%w", err)
	}
	return err
}

func runGain(cmd *cobra.Command, args []string) error {
	since := time.Now().AddDate(0, 0, -30)
	sum, err := proxy.LoadGainSummary(since)
	if err != nil {
		return err
	}
	switch flagFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"window_days": 30,
			"summary":     sum,
		})
	case "table":
		fmt.Fprintf(os.Stdout, "window: last %d days\n", 30)
		fmt.Fprintf(os.Stdout, "proxy commands: %d\n", sum.Records)
		fmt.Fprintf(os.Stdout, "raw tokens: %d\n", sum.RawTokens)
		fmt.Fprintf(os.Stdout, "compact tokens: %d\n", sum.CompactTokens)
		fmt.Fprintf(os.Stdout, "tokens saved: %d (%s%%)\n", sum.TokensSaved, strconv.FormatFloat(sum.SavedPercent, 'f', 1, 64))
		if sum.Records == 0 {
			fmt.Fprintln(os.Stdout, "note: no proxied shell commands recorded yet")
			fmt.Fprintln(os.Stdout, "tip: run commands via 'billy proxy -- <cmd>' or enable hook rewrite with 'billy init'")
		} else if sum.Records < 5 {
			fmt.Fprintln(os.Stdout, "note: low sample size; run more proxied shell commands for stable savings stats")
		}
		fmt.Fprintln(os.Stdout, "tracked file: ~/.billy/proxy-usage.jsonl")
		return nil
	default:
		return fmt.Errorf("unknown format %q", flagFormat)
	}
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
				fmt.Fprintf(os.Stderr, "billy: no session for project %s; using latest Claude Code log: %s\n", abs, sessionPath)
			} else if pn == provider.Auto {
				fmt.Fprintf(os.Stderr, "billy: no Claude session for project %s; using latest %s log: %s\n", abs, provider.VendorLabel(v), sessionPath)
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

func runForecast(path string) error {
	if flagWeeklyLimitUSD > 0 && flagWeeklyLimitTokens > 0 {
		return fmt.Errorf("use only one of --weekly-limit-usd and --weekly-limit-tokens")
	}
	var metric analyze.ForecastMetric
	var limit float64
	switch {
	case flagWeeklyLimitUSD > 0:
		metric = analyze.ForecastUSD
		limit = flagWeeklyLimitUSD
	case flagWeeklyLimitTokens > 0:
		metric = analyze.ForecastTokens
		limit = float64(flagWeeklyLimitTokens)
	default:
		return fmt.Errorf("set --weekly-limit-usd or --weekly-limit-tokens")
	}

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

	now := time.Now()
	var answers []model.AnswerReport

	if st.IsDir() && !flagLatestOnly {
		answers, err = collectAnswersAggregate(pn, pt, flagAgents)
		if err != nil {
			return err
		}
	} else {
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
					fmt.Fprintf(os.Stderr, "billy: no session for project %s; using latest Claude Code log: %s\n", abs, sessionPath)
				} else if pn == provider.Auto {
					fmt.Fprintf(os.Stderr, "billy: no Claude session for project %s; using latest %s log: %s\n", abs, provider.VendorLabel(v), sessionPath)
				}
			}
			pn = provider.ProviderName(v)
		}
		vendor, events, err := provider.ReadSession(pn, sessionPath)
		if err != nil {
			return err
		}
		rep := analyze.BuildReport(sessionPath, vendor, events, pt)
		answers = rep.Answers
	}

	f := analyze.ForecastWeeklyLimit(answers, now, limit, metric)
	return emitForecast(f)
}

func collectAnswersAggregate(pn provider.Name, pt *pricing.Table, agents bool) ([]model.AnswerReport, error) {
	var out []model.AnswerReport
	appendFrom := func(paths []string, v model.Vendor, read func(string) ([]model.NormalizedEvent, error)) {
		for _, p := range paths {
			ev, err := read(p)
			if err != nil {
				fmt.Fprintf(os.Stderr, "billy: skip %s: %v\n", p, err)
				continue
			}
			rep := analyze.BuildReport(p, v, ev, pt)
			out = append(out, rep.Answers...)
		}
	}
	switch pn {
	case provider.Auto:
		appendFrom(anthropic.AllAnthropicSessionPaths(agents), model.VendorAnthropic, anthropic.ReadEvents)
		appendFrom(codex.CollectSessionJSONLPaths(), model.VendorOpenAI, codex.ReadEvents)
		appendFrom(cursor.CollectWorkspaceStateDBs(), model.VendorCursor, cursorReadSQLite)
	case provider.Anthropic:
		appendFrom(anthropic.AllAnthropicSessionPaths(agents), model.VendorAnthropic, anthropic.ReadEvents)
	case provider.OpenAI:
		appendFrom(codex.CollectSessionJSONLPaths(), model.VendorOpenAI, codex.ReadEvents)
	case provider.CursorProv:
		appendFrom(cursor.CollectWorkspaceStateDBs(), model.VendorCursor, cursorReadSQLite)
	default:
		return nil, fmt.Errorf("unknown provider %q", pn)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no session files found for provider %q (try --latest-only or a different --provider)", pn)
	}
	return out, nil
}

func emitForecast(f analyze.WeeklyForecast) error {
	loc := f.Now.Location()
	switch flagFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(f.ReportRFC3339(loc))
	case "table":
		output.PrintForecast(os.Stdout, f, displayOptions())
		return nil
	default:
		return fmt.Errorf("unknown format %q", flagFormat)
	}
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
			fmt.Fprintf(os.Stderr, "billy: skip %s: %v\n", p, err)
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

func runHeatmap(path string) error {
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

	var answers []model.AnswerReport
	if st.IsDir() && !flagLatestOnly {
		answers, err = collectAnswersAggregate(pn, pt, flagAgents)
		if err != nil {
			return err
		}
	} else {
		sessionPath := abs
		if st.IsDir() {
			var v model.Vendor
			var usedFallback bool
			sessionPath, v, usedFallback, err = provider.ResolveDirToSession(pn, abs, flagAgents)
			if err != nil {
				return err
			}
			if usedFallback && (pn == provider.Auto || pn == provider.Anthropic) {
				fmt.Fprintf(os.Stderr, "billy: no session for project %s; using latest log: %s\n", abs, sessionPath)
			}
			pn = provider.ProviderName(v)
		}
		_, events, err := provider.ReadSession(pn, sessionPath)
		if err != nil {
			return err
		}
		rep := analyze.BuildReport(sessionPath, model.Vendor(pn), events, pt)
		answers = rep.Answers
	}

	weeks := flagHeatmapWeeks
	if flagHeatmapMonthly {
		weeks = 4
	}
	if weeks <= 0 {
		weeks = 52
	}

	now := time.Now().In(time.Local)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	from := today.AddDate(0, 0, -(weeks*7 - 1))

	metric := flagHeatmapMetric
	if metric != output.HeatmapMetricCost && metric != output.HeatmapMetricTokens {
		metric = output.HeatmapMetricCost
	}

	buckets := analyze.BucketByDay(answers, from, today)
	output.PrintHeatmap(os.Stdout, buckets, displayOptions(), output.HeatmapOptions{
		Metric: metric,
		Weeks:  weeks,
	})
	return nil
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
