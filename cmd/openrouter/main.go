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
		return time.Time{}, fmt.Errorf("--since должен быть RFC3339 или датой YYYY-MM-DD: %w", err)
	}
	return parsed, nil
}

func renderHistory(h *pricehistory.History, modelSlug, since, format string) (string, error) {
	if format != "markdown" && format != "tsv" {
		return "", fmt.Errorf("--format должен быть markdown или tsv")
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
		cfgPath string
		dataDir string
		output  string
		dryRun  bool
	)

	root := &cobra.Command{
		Use:     "openrouter",
		Version: version,
		Short:   "Пересобирает сравнение моделей OpenRouter из живых цен и бенчмарк-лидербордов",
		Long: fmt.Sprintf("Version: %s\n\n", version) +
			"openrouter собирает цены и контекст с публичного OpenRouter API, оценки — со swebench.com\n" +
			"и vals.ai по ручной карте model-map.tsv, и целиком перегенерирует markdown-документ.\n" +
			"Проза живёт в notes.yaml: правки в самом .md следующий прогон затрёт.",
		SilenceUsage: true,
	}
	root.PersistentFlags().StringVar(&cfgPath, "config", config.DefaultPath(), "путь к config.yaml")
	root.PersistentFlags().StringVar(&dataDir, "data-dir", "", "каталог проекта с model-map.tsv, notes.yaml и cache/ (перекрывает конфиг)")

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
		Use:   "refresh",
		Short: "Собрать свежие данные и перезаписать документ",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return runRefresh(cmd, dryRun) },
	}
	refreshCmd.Flags().StringVar(&output, "output", "", "путь генерируемого markdown (перекрывает конфиг)")
	refreshCmd.Flags().BoolVar(&dryRun, "dry-run", false, "ничего не писать: ни документ, ни снимок")

	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "Только отчёт: новые кандидаты, снятые slug'и, пробелы в notes.yaml — без записи",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return runRefresh(cmd, true) },
	}
	checkCmd.Flags().StringVar(&output, "output", "", "путь генерируемого markdown (нужен только для проверки конфига)")

	var historyModel, historySince, historyFormat string
	historyCmd := &cobra.Command{
		Use:   "history",
		Short: "Показать историю цен",
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
	historyCmd.Flags().StringVar(&historyModel, "model", "", "фильтр по slug")
	historyCmd.Flags().StringVar(&historySince, "since", "", "показывать наблюдения после RFC3339 или даты YYYY-MM-DD")
	historyCmd.Flags().StringVar(&historyFormat, "format", "markdown", "формат: markdown или tsv")

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Показать версию бинарника",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "openrouter %s\n", version)
		},
	}

	root.AddCommand(refreshCmd, checkCmd, historyCmd, versionCmd)
	return root
}
