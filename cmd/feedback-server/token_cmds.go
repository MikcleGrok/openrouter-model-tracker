package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// newTokenCmd builds `feedback-server token init --token-file PATH` (plan
// 6.1/7.2): the server-side half of user-token provisioning. There is
// deliberately no `token rotate` here — the brief names only `token init`
// for the user secret; rotating it is described (plan 6.1) as "an external
// secure provisioning operation" the operator performs, not a dedicated
// subcommand this task introduces.
func newTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Manage the shared user bearer token secret",
	}

	var tokenFile string
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Create a new user token secret file (refuses to overwrite an existing one)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTokenInit(tokenFile, cmd.OutOrStdout())
		},
	}
	initCmd.Flags().StringVar(&tokenFile, "token-file", "", "path to create")
	_ = initCmd.MarkFlagRequired("token-file")

	cmd.AddCommand(initCmd)
	return cmd
}

func runTokenInit(path string, out io.Writer) error {
	secret, err := generateSecretHex()
	if err != nil {
		return err
	}
	if err := writeSecretFileExclusive(path, secret); err != nil {
		return err
	}
	fmt.Fprintf(out, "feedback-server: created user token file %s\n", path)
	return nil
}

// newConsumerTokenCmd builds `feedback-server consumer-token init|rotate
// --consumer-token-file PATH` (plan 6.1/6.3/7.2): provisioning and rotation
// for the separate trusted-consumer credential. It never touches the user
// token-file (plan 6.1: "Consumer credential полностью отделён от user
// token-file"; plan 7.2: "Отдельные команды provisioning/rotation
// consumer credential не переиспользуют user token-file").
func newConsumerTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "consumer-token",
		Short: "Manage the separate trusted-consumer bearer token secret",
	}

	var initFile string
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Create a new consumer token secret file (refuses to overwrite an existing one)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConsumerTokenInit(initFile, cmd.OutOrStdout())
		},
	}
	initCmd.Flags().StringVar(&initFile, "consumer-token-file", "", "path to create")
	_ = initCmd.MarkFlagRequired("consumer-token-file")

	var rotateFile string
	rotateCmd := &cobra.Command{
		Use:   "rotate",
		Short: "Atomically replace the consumer token secret with a fresh one",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConsumerTokenRotate(rotateFile, cmd.OutOrStdout())
		},
	}
	rotateCmd.Flags().StringVar(&rotateFile, "consumer-token-file", "", "path to replace")
	_ = rotateCmd.MarkFlagRequired("consumer-token-file")

	cmd.AddCommand(initCmd, rotateCmd)
	return cmd
}

func runConsumerTokenInit(path string, out io.Writer) error {
	secret, err := generateSecretHex()
	if err != nil {
		return err
	}
	if err := writeSecretFileExclusive(path, secret); err != nil {
		return err
	}
	fmt.Fprintf(out, "feedback-server: created consumer token file %s\n", path)
	return nil
}

func runConsumerTokenRotate(path string, out io.Writer) error {
	secret, err := generateSecretHex()
	if err != nil {
		return err
	}
	if err := writeSecretFileAtomic(path, secret); err != nil {
		return err
	}
	fmt.Fprintf(out, "feedback-server: rotated consumer token file %s; restart feedback-server (and any consumer) to pick it up\n", path)
	return nil
}
