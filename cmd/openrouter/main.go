// Command openrouter regenerates docs/openrouter-model-comparison.md from live
// OpenRouter prices and benchmark leaderboards.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/sboborikin/openrouter-model-tracker/internal/config"
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

func newRootCmd() *cobra.Command {
	var (
		cfgPath string
		dataDir string
		output  string
		dryRun  bool
	)

	root := &cobra.Command{
		Use:   "openrouter",
		Short: "Пересобирает сравнение моделей OpenRouter из живых цен и бенчмарк-лидербордов",
		Long: "openrouter собирает цены и контекст с публичного OpenRouter API, оценки — со swebench.com\n" +
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
		if err != nil {
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

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Показать версию бинарника",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "openrouter %s\n", version)
		},
	}

	root.AddCommand(refreshCmd, checkCmd, versionCmd)
	return root
}
