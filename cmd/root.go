package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "arh",
	Short: "AI Review Helper — pre-review triage for GitHub PRs",
	Long: `arh fetches a GitHub PR, runs multiple analysis agents (rules, lint, logic, focus),
and produces a structured report to help you focus your review.`,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringP("config", "c", "", "path to config file (default: .arh.yaml)")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "show agent execution details")

	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintln(os.Stdout, "arh version 0.1.0")
	},
}
