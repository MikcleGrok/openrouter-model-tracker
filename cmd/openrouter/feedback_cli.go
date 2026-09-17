package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sboborikin/openrouter-model-tracker/internal/config"
	feedbackclient "github.com/sboborikin/openrouter-model-tracker/internal/feedback/client"
)

// newFeedbackCmd builds `openrouter feedback init` (plan 6.1/7.3): the
// client-side half of feedback identity provisioning. It is the counterpart
// of cmd/feedback-server's `token init`/`consumer-token init` commands, but
// deliberately does not generate or copy any secret itself — see
// runFeedbackInit's own doc comment. cfgPath is newRootCmd's own --config
// persistent-flag variable, read at RunE time (after cobra has parsed
// flags), the same way every other subcommand in this file closes over it.
func newFeedbackCmd(cfgPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "feedback",
		Short: "Manage the local identity for the optional feedback-server integration",
	}

	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Create the local feedback identity and verify the token file is readable",
		Long: "Create the local feedback identity and verify the token file is readable.\n\n" +
			"init creates feedback.identity_file if it does not yet hold a valid identity (reusing\n" +
			"it unchanged otherwise) and verifies feedback.token_file is readable. It never\n" +
			"generates or copies the shared token secret itself — that is a separate, server-side\n" +
			"provisioning step: run 'feedback-server token init --token-file PATH' first, then\n" +
			"point feedback.token_file at that same PATH (directly, or via --config). See\n" +
			"docs/reference.md's Feedback section for the full local setup sequence.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runFeedbackInit(*cfgPath, cmd.OutOrStdout())
		},
	}
	cmd.AddCommand(initCmd)

	var deleteYes bool
	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Permanently delete this identity's feedback data from the server",
		Long: "Permanently delete this identity's feedback data from the server.\n\n" +
			"delete calls DELETE /v1/me/feedback (internal/feedback/client.DeleteMe) for the\n" +
			"identity in feedback.identity_file: every rating and review this identity ever\n" +
			"submitted is removed on the server and cannot be recovered. It asks for an\n" +
			"explicit y/yes confirmation on stdin before deleting anything; pass --yes/-y to\n" +
			"skip the prompt for scripted use. A 'pending' response means the delete already\n" +
			"committed and only the server's background cleanup is still finishing — re-run\n" +
			"the command later (it is idempotent) to confirm it is fully done.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runFeedbackDelete(*cfgPath, deleteYes, cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	deleteCmd.Flags().BoolVarP(&deleteYes, "yes", "y", false, "skip the confirmation prompt")
	cmd.AddCommand(deleteCmd)

	return cmd
}

// runFeedbackInit loads cfgPath, resolves feedback.token_file/identity_file
// the same config-relative way withFeedbackClient already does
// (resolveConfigPath), and delegates to feedbackclient.Init — the tested
// internal/feedback/client operation that creates/reuses the identity and
// checks the token file, without ever reading or printing its content. A
// config with no "feedback:" section at all still works: config.Load
// defaults every field, so init runs against
// DefaultFeedbackTokenFile/DefaultFeedbackIdentityFile.
func runFeedbackInit(cfgPath string, out io.Writer) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	tokenFile := resolveConfigPath(cfgPath, cfg.Feedback.EffectiveTokenFile())
	identityFile := resolveConfigPath(cfgPath, cfg.Feedback.EffectiveIdentityFile())
	result, err := feedbackclient.Init(identityFile, tokenFile)
	if err != nil {
		return err
	}
	if result.IdentityCreated {
		fmt.Fprintf(out, "Created: %s (identity %s)\n", identityFile, result.IdentityID)
	} else {
		fmt.Fprintf(out, "Already exists: %s (identity %s)\n", identityFile, result.IdentityID)
	}
	fmt.Fprintf(out, "Token file OK: %s\n", tokenFile)
	if !cfg.Feedback.Enabled {
		fmt.Fprintln(out, "Note: feedback.enabled is false in the config; the TUI's Feedback tab stays off until you set it to true.")
	}
	return nil
}

// feedbackClientFromConfig loads cfgPath and constructs a
// *feedbackclient.Client from its feedback section, resolving
// token_file/identity_file the same config-relative way runFeedbackInit and
// withFeedbackClient (feedback.go, the TUI's own client construction) already
// do (resolveConfigPath). Shared by every feedback subcommand that needs to
// actually talk to the server, not just the ones (like init) that only
// touch local files.
func feedbackClientFromConfig(cfgPath string) (*feedbackclient.Client, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}
	timeout, err := cfg.Feedback.EffectiveRequestTimeout()
	if err != nil {
		return nil, err
	}
	tokenFile := resolveConfigPath(cfgPath, cfg.Feedback.EffectiveTokenFile())
	identityFile := resolveConfigPath(cfgPath, cfg.Feedback.EffectiveIdentityFile())
	return feedbackclient.New(feedbackclient.Config{
		Endpoint:       cfg.Feedback.EffectiveEndpoint(),
		TokenFile:      tokenFile,
		IdentityFile:   identityFile,
		RequestTimeout: timeout,
	})
}

// runFeedbackDelete implements `openrouter feedback delete`: unless
// skipConfirm is set, it asks for an explicit confirmation on in before doing
// anything irreversible, then builds a feedback client from cfgPath
// (feedbackClientFromConfig) and calls DeleteMe, reporting its three-way,
// already-reviewed outcome (internal/feedback/client.DeleteMe's own doc
// comment): 204 done, 202 cleanup_pending (committed; safe and expected to
// re-run later), or an error (nothing committed).
func runFeedbackDelete(cfgPath string, skipConfirm bool, in io.Reader, out io.Writer) error {
	if !skipConfirm {
		confirmed, err := confirmFeedbackDelete(in, out)
		if err != nil {
			return fmt.Errorf("read confirmation: %w", err)
		}
		if !confirmed {
			fmt.Fprintln(out, "Aborted: feedback data was not deleted.")
			return nil
		}
	}

	client, err := feedbackClientFromConfig(cfgPath)
	if err != nil {
		return err
	}
	result, err := client.DeleteMe(context.Background())
	if err != nil {
		return fmt.Errorf("delete feedback data: %w", err)
	}
	if result.Pending {
		fmt.Fprintln(out, "Committed: the delete has been recorded on the server; background cleanup is still finishing. It is safe to re-run this command to confirm cleanup is fully done.")
		return nil
	}
	fmt.Fprintln(out, "Deleted: all feedback data for this identity has been removed from the server.")
	return nil
}

// confirmFeedbackDelete prints a destructive-action warning to out and reads
// one line from in, reporting true only for an explicit "y" or "yes"
// (case-insensitive, surrounding whitespace trimmed). Anything else —
// including a bare Enter, EOF, or a scan error — is treated as "no": a
// genuinely destructive, irreversible action like this one has no room for
// a permissive default.
func confirmFeedbackDelete(in io.Reader, out io.Writer) (bool, error) {
	fmt.Fprint(out, "This permanently deletes all feedback data for this identity and cannot be undone. Continue? [y/N] ")
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return false, scanner.Err()
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes", nil
}
