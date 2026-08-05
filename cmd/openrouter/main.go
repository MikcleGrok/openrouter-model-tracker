// Command openrouter regenerates docs/openrouter-model-comparison.md from live
// OpenRouter prices and benchmark leaderboards.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/sboborikin/openrouter-model-tracker/internal/config"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricehistory"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricing"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "openrouter:", err)
		os.Exit(1)
	}
}

// resolveOptions merges the config file with the flags. A flag always wins.
func resolveOptions(cfgPath, dataDir, output string) (refresh.Options, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return refresh.Options{}, err
	}
	opts := refresh.Options{DataDir: cfg.DataDir, OutputPath: cfg.DefaultOutput}
	if dataDir != "" {
		opts.DataDir = dataDir
	}
	if output != "" {
		opts.OutputPath = output
	}
	if opts.DataDir == "" {
		return opts, errors.New("нет каталога данных: передай --data-dir или задай data_dir в конфиге")
	}
	if opts.OutputPath == "" {
		return opts, errors.New("нет пути вывода: передай --output или задай default_output в конфиге")
	}
	return opts, nil
}

func resolveDataDir(cfgPath, dataDir string) (string, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return "", err
	}
	if dataDir != "" {
		return dataDir, nil
	}
	if cfg.DataDir == "" {
		return "", errors.New("нет каталога данных: передай --data-dir или задай data_dir в конфиге")
	}
	return cfg.DataDir, nil
}

func parseSince(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("--since must be RFC3339 or YYYY-MM-DD: %w", err)
	}
	return parsed, nil
}

func renderHistory(h *pricehistory.History, modelSlug, since, format string) (string, error) {
	if format != "markdown" && format != "tsv" {
		return "", fmt.Errorf("--format must be markdown or tsv")
	}
	cutoff, err := parseSince(since)
	if err != nil {
		return "", err
	}
	previous := make(map[string]pricehistory.Price)
	var b strings.Builder
	if format == "markdown" {
		b.WriteString("| timestamp | slug | input $/M | output $/M | context | long-context threshold | override input $/M | override output $/M | change |\n|---|---|---:|---:|---:|---:|---:|---:|---|\n")
	} else {
		b.WriteString("timestamp\tslug\tinput_per_million\toutput_per_million\tcontext\tlong_context_min_tokens\tlong_context_input_per_million\tlong_context_output_per_million\tchange\n")
	}
	rows := 0
	for _, observation := range h.Observations {
		if !cutoff.IsZero() && observation.ObservedAt.Before(cutoff) {
			for slug, price := range observation.Prices {
				previous[slug] = price
			}
			continue
		}
		slugs := make([]string, 0, len(observation.Prices))
		for slug := range observation.Prices {
			if modelSlug == "" || modelSlug == slug {
				slugs = append(slugs, slug)
			}
		}
		sort.Strings(slugs)
		for _, slug := range slugs {
			price := observation.Prices[slug]
			change := ""
			if before, ok := previous[slug]; ok && !pricehistory.Equal(before, price) {
				change = fmt.Sprintf("%s -> %s", pricehistory.Format(before), pricehistory.Format(price))
			}
			if format == "markdown" {
				threshold, overrideIn, overrideOut := "", "", ""
				if price.HasOverride {
					threshold = pricing.FormatContext(price.OverrideMinTokens) + "+"
					overrideIn, overrideOut = pricing.FormatPrice(price.OverrideInPerM), pricing.FormatPrice(price.OverrideOutPerM)
				}
				b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %d | %s | %s | %s | %s |\n", observation.ObservedAt.UTC().Format(time.RFC3339), slug, pricing.FormatPrice(price.InPerM), pricing.FormatPrice(price.OutPerM), price.Context, threshold, overrideIn, overrideOut, change))
			} else {
				threshold, overrideIn, overrideOut := "", "", ""
				if price.HasOverride {
					threshold = strconv.Itoa(price.OverrideMinTokens)
					overrideIn, overrideOut = strconv.FormatFloat(price.OverrideInPerM, 'f', -1, 64), strconv.FormatFloat(price.OverrideOutPerM, 'f', -1, 64)
				}
				b.WriteString(strings.Join([]string{observation.ObservedAt.UTC().Format(time.RFC3339), slug, strconv.FormatFloat(price.InPerM, 'f', -1, 64), strconv.FormatFloat(price.OutPerM, 'f', -1, 64), strconv.Itoa(price.Context), threshold, overrideIn, overrideOut, change}, "\t") + "\n")
			}
			rows++
		}
		for slug, price := range observation.Prices {
			previous[slug] = price
		}
	}
	if rows == 0 {
		return "История цен пуста или не содержит подходящих наблюдений.\n", nil
	}
	return b.String(), nil
}

func newRootCmd() *cobra.Command {
	var (
		cfgPath       string
		dataDir       string
		output        string
		dryRun        bool
		tableSort     string
		tableReverse  bool
		tableLimit    int
		tableNoPager  bool
		tableShowSlug bool
		tableFilters  []string
	)

	root := &cobra.Command{
		Use:     "openrouter",
		Version: version,
		Short:   "Regenerate the OpenRouter model comparison from live prices and benchmark leaderboards",
		Long: fmt.Sprintf("Version: %s\n\n", version) +
			"openrouter collects prices and context from the public OpenRouter API, and scores from swebench.com\n" +
			"and vals.ai using the manual model-map.tsv mapping, then regenerates the markdown document.\n" +
			"Prose lives in notes.yaml: edits to the .md file itself will be overwritten on the next run.",
		SilenceUsage: true,
	}
	root.PersistentFlags().StringVar(&cfgPath, "config", config.DefaultPath(), "path to config.yaml")
	root.PersistentFlags().StringVar(&dataDir, "data-dir", "", "project directory with model-map.tsv, notes.yaml, and cache/ (overrides config)")

	runRefresh := func(cmd *cobra.Command, dry bool) error {
		opts, err := resolveOptions(cfgPath, dataDir, output)
		if err != nil {
			return err
		}
		opts.DryRun = dry
		report, err := refresh.Run(cmd.Context(), opts)
		// Print the report exactly once: on error, only if there is
		// something in it to show (Warnings survive even a hard failure);
		// on success, always.
		if err != nil {
			if refresh.IsPostCommitCleanupError(err) {
				fmt.Fprintln(cmd.OutOrStdout(), "⚠️ Данные опубликованы, но очистка временных и резервных файлов не завершилась.")
			}
			if len(report.Warnings) > 0 {
				fmt.Fprint(cmd.OutOrStdout(), report.String())
			}
			return err
		}
		fmt.Fprint(cmd.OutOrStdout(), report.String())
		if !dry {
			fmt.Fprintf(cmd.OutOrStdout(), "📄 Записано: %s\n", opts.OutputPath)
		}
		return nil
	}

	refreshCmd := &cobra.Command{
		Use:     "refresh",
		Aliases: []string{"update", "up"},
		Short:   "Fetch fresh data and overwrite the document",
		Args:    cobra.NoArgs,
		RunE:    func(cmd *cobra.Command, _ []string) error { return runRefresh(cmd, dryRun) },
	}
	refreshCmd.Flags().StringVar(&output, "output", "", "path to generated markdown (overrides config)")
	refreshCmd.Flags().BoolVar(&dryRun, "dry-run", false, "write nothing: neither the document nor the snapshot")

	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "Report only: new candidates, removed slugs, and notes.yaml gaps; write nothing",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return runRefresh(cmd, true) },
	}
	checkCmd.Flags().StringVar(&output, "output", "", "path to generated markdown (used only to validate config)")

	var historyModel, historySince, historyFormat string
	historyCmd := &cobra.Command{
		Use:   "history",
		Short: "Show price history",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := resolveDataDir(cfgPath, dataDir)
			if err != nil {
				return err
			}
			history, err := pricehistory.Load(pricehistory.Path(dir))
			if err != nil {
				return err
			}
			output, err := renderHistory(history, historyModel, historySince, historyFormat)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), output)
			return nil
		},
	}
	historyCmd.Flags().StringVar(&historyModel, "model", "", "filter by slug")
	historyCmd.Flags().StringVar(&historySince, "since", "", "show observations after RFC3339 or YYYY-MM-DD")
	historyCmd.Flags().StringVar(&historyFormat, "format", "markdown", "format: markdown or tsv")

	tableCmd := &cobra.Command{
		Use:                "table",
		Short:              "Show local model data as a plain-text table",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
				return cmd.Help()
			}
			if err := parseTableArgs(args, cmd.Flags()); err != nil {
				return err
			}
			if cmd.Flags().Changed("limit") && tableLimit < 0 {
				return fmt.Errorf("table: limit must be non-negative, got %d", tableLimit)
			}
			dir, err := resolveDataDir(cfgPath, dataDir)
			if err != nil {
				return err
			}
			models, err := loadLocalModels(dir)
			if err != nil {
				return err
			}
			models, err = filterTableModels(models, tableFilters)
			if err != nil {
				return err
			}
			if err := sortTableModels(models, tableSort, tableReverse); err != nil {
				return err
			}
			models = limitTableModels(models, tableLimit)
			width, err := tableWidth()
			if err != nil {
				return err
			}
			shouldPage := tableShouldPage(cmd.OutOrStdout(), tableNoPager)
			return writeTableOutput(renderTable(models, width, tableShowSlug), cmd.OutOrStdout(), cmd.ErrOrStderr(), shouldPage)
		},
	}
	tableCmd.Flags().StringVarP(&tableSort, "sort", "s", "q/p", "sort by: "+tableSortHelp)
	tableCmd.Flags().BoolVarP(&tableReverse, "reverse", "R", false, "reverse the primary sort order")
	tableCmd.Flags().IntVarP(&tableLimit, "limit", "n", -1, "show only the first N models after sorting; 0 means unlimited; standalone -N is shorthand for -n N")
	tableCmd.Flags().StringArrayVarP(&tableFilters, "filter", "f", nil, "filter: paid, free, scored, tier:*, quality>=N, context>=N, input<=N, output<=N (repeatable, AND)")
	tableCmd.Flags().BoolVar(&tableNoPager, "no-pager", false, "do not use less in a TTY")
	tableCmd.Flags().BoolVarP(&tableShowSlug, "slug", "S", false, "show Slug instead of Name as the first column")

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Show the binary version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "openrouter %s\n", version)
		},
	}

	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Create a user config and local cache directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			items, err := config.Init(cfgPath, dataDir)
			if err != nil {
				return err
			}
			for _, item := range items {
				fmt.Fprintln(cmd.OutOrStdout(), item)
			}
			return nil
		},
	}

	root.AddCommand(refreshCmd, checkCmd, historyCmd, versionCmd, initCmd)
	root.AddCommand(tableCmd)
	return root
}
