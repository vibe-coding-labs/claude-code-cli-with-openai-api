package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Long:  `Display the version, build time, and git commit information for the application.`,
	Run: func(cmd *cobra.Command, args []string) {
		printVersion()
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

func printVersion() {
	color.New(color.FgCyan, color.Bold).Println("Claude-to-OpenAI API Proxy (Golang)")
	fmt.Println()

	color.New(color.FgGreen, color.Bold).Print("Version:     ")
	color.New(color.FgWhite).Printf("%s\n", Version)

	color.New(color.FgGreen, color.Bold).Print("Build Time:  ")
	color.New(color.FgWhite).Printf("%s\n", BuildTime)

	color.New(color.FgGreen, color.Bold).Print("Git Commit:  ")
	color.New(color.FgWhite).Printf("%s\n", GitCommit)

	fmt.Println()
	color.New(color.FgYellow).Println("For more information, visit: https://github.com/vibe-coding-labs/claude-code-cli-with-openai-api")
}
