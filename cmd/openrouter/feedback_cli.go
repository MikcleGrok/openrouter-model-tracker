package main

import (
	"fmt"
	"io"

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
