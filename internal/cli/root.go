package cli

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewRootCmd creates the root command
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "repogen",
		Short: "Generate static repository structures for multiple package managers",
		Long: `Repogen scans directories for package files and generates static
repository structures that can be served as websites.

Supported package types:
  - Debian/APT (.deb packages)
  - Yum/RPM (.rpm packages)
  - Alpine/APK (.apk packages)
  - Homebrew (bottle files)`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Setup logging
			verbose, _ := cmd.Flags().GetBool("verbose")
			if verbose {
				logrus.SetLevel(logrus.DebugLevel)
			} else {
				logrus.SetLevel(logrus.InfoLevel)
			}
		},
	}

	// Global flags
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose logging")

	// Add subcommands
	rootCmd.AddCommand(NewGenerateCmd())

	return rootCmd
}
