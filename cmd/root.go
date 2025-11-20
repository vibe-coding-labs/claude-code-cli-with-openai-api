package cmd

import (
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	Version   = "1.0.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "claude-with-openai-api",
	Short: "Use ClaudeCode CLI With OpenAI API",
	Long: `Use ClaudeCode CLI With OpenAI API

A high-performance proxy server that translates Claude API requests to OpenAI API format.

Features:
  • Seamless Claude API compatibility
  • Automatic model mapping (haiku/sonnet/opus → GPT models)
  • Streaming and non-streaming support
  • Token counting support
  • Health monitoring

Use "claude-with-openai-api [command] --help" for more information about a command.`,
	Version: Version,
	// Default behavior: run server if no subcommand provided
	// This is handled by making server the default in Execute()
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	// Check if a subcommand was provided
	// If not, and it's not help/version, default to server
	hasSubcommand := false
	if len(os.Args) > 1 {
		firstArg := os.Args[1]
		// Check if it's a flag (starts with -) or a subcommand
		if firstArg[0] != '-' {
			hasSubcommand = true
		} else if firstArg == "--help" || firstArg == "-h" || firstArg == "--version" || firstArg == "-v" {
			hasSubcommand = false // Let cobra handle these
		}
	}

	// If no subcommand and not help/version, default to server
	if !hasSubcommand && len(os.Args) == 1 {
		os.Args = append(os.Args, "server")
	}

	if err := rootCmd.Execute(); err != nil {
		color.New(color.FgRed, color.Bold).Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Add version template with colors
	rootCmd.SetVersionTemplate(color.New(color.FgGreen, color.Bold).Sprintf("Version: %s\n", Version) +
		color.New(color.FgWhite).Sprintf("Build Time: %s\n", BuildTime) +
		color.New(color.FgWhite).Sprintf("Git Commit: %s\n", GitCommit))

	// Customize help and usage templates with colors
	cobra.AddTemplateFunc("style", func(style string, text string) string {
		switch style {
		case "cyan":
			return color.CyanString(text)
		case "yellow":
			return color.YellowString(text)
		case "green":
			return color.GreenString(text)
		case "bold":
			return color.New(color.Bold).Sprint(text)
		default:
			return text
		}
	})
}
