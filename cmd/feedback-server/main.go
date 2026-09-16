// Command feedback-server is the standalone process that owns the feedback
// SQLite database and answers internal/feedback/httpapi requests (plan
// 3.1). It does not import cmd/openrouter and knows nothing about the TUI;
// the TUI is a separate HTTP client of this process (a later task).
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "feedback-server:", err)
		os.Exit(1)
	}
}

// defaultDBPath and defaultBackupDir mirror internal/config.DefaultPath's
// own "user home, else a relative fallback" pattern (cmd/openrouter's
// config path), giving --db/--backup-dir an explicit, safe default in the
// user's data directory (plan 7.2: "--db, обязательный либо с явным
// безопасным default в user data dir") rather than defaulting into the
// current working directory.
func defaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".local", "share", "openrouter-feedback", "feedback.db")
	}
	return filepath.Join(home, ".local", "share", "openrouter-feedback", "feedback.db")
}

func defaultBackupDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".local", "share", "openrouter-feedback", "backups")
	}
	return filepath.Join(home, ".local", "share", "openrouter-feedback", "backups")
}

func newRootCmd() *cobra.Command {
	var cfg runConfig

	root := &cobra.Command{
		Use:   "feedback-server",
		Short: "Run the feedback HTTP API server",
		Long: "feedback-server opens the feedback SQLite database, applies migrations, and listens for\n" +
			"internal/feedback/httpapi requests on a loopback address by default. It is a separate\n" +
			"process from the openrouter TUI/CLI: no data or SQL schema is shared in-process.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServer(cmd.Context(), cfg, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	root.Flags().StringVar(&cfg.listen, "listen", "127.0.0.1:8787", "address to listen on")
	root.Flags().StringVar(&cfg.dbPath, "db", defaultDBPath(), "path to the SQLite database")
	root.Flags().StringVar(&cfg.tokenFile, "token-file", "", "path to the user bearer token secret file (required; create with 'feedback-server token init')")
	root.Flags().StringVar(&cfg.consumerTokenFile, "consumer-token-file", "", "path to the trusted-consumer bearer token secret file (required; create with 'feedback-server consumer-token init')")
	root.Flags().StringVar(&cfg.backupDir, "backup-dir", defaultBackupDir(), "directory for DELETE /v1/me/feedback's post-commit backups")
	root.Flags().StringVar(&cfg.logLevel, "log-level", "info", "log level: debug, info, warn, or error")
	_ = root.MarkFlagRequired("token-file")
	_ = root.MarkFlagRequired("consumer-token-file")

	root.AddCommand(newTokenCmd(), newConsumerTokenCmd())
	return root
}
