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

// shouldDefaultToServer reports whether Execute should inject the "server"
// subcommand into args (args[0] is the program name, matching os.Args).
// It returns false when args already name an explicit subcommand (e.g.
// "config", "migrate"), or when the first argument is a help/version flag
// that cobra should handle itself on the root command. It returns true for
// a completely bare invocation as well as a flag-only invocation (e.g.
// "claude-with-openai-api --port 8080"), so that flags meant for the server
// command work without spelling out "server" explicitly.
func shouldDefaultToServer(args []string) bool {
	if len(args) <= 1 {
		return true
	}
	firstArg := args[1]
	if firstArg == "" {
		return true
	}
	if firstArg[0] != '-' {
		// An explicit subcommand was given.
		return false
	}
	switch firstArg {
	case "--help", "-h", "--version", "-v":
		return false
	}
	return true
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if shouldDefaultToServer(os.Args) {
		args := make([]string, 0, len(os.Args)+1)
		args = append(args, os.Args[0], "server")
		args = append(args, os.Args[1:]...)
		os.Args = args
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
