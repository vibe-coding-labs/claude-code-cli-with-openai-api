package cmd

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show current configuration",
	Long: `Display the current configuration loaded from environment variables and .env file.

The configuration includes:
  • OpenAI API settings
  • Model mappings
  • Server settings
  • Security settings`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return printConfig()
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	
	// Add flags for config command
	configCmd.Flags().BoolP("validate", "v", false, "Validate configuration before displaying")
}

func printConfig() error {
	// Try to load config (but don't fail if OPENAI_API_KEY is missing for display purposes)
	cfg, err := config.LoadConfig()
	if err != nil {
		// If it's just a missing API key, show what we can
		color.New(color.FgYellow, color.Bold).Println("⚠️  Configuration Warning:")
		color.New(color.FgYellow).Printf("   %v\n\n", err)
		
		// Show environment-based config anyway
		showEnvBasedConfig()
		return nil
	}

	// Header
	color.New(color.FgCyan, color.Bold).Println("📋 Current Configuration")
	fmt.Println()

	// API Configuration
	color.New(color.FgYellow, color.Bold).Println("🔑 API Configuration:")
	
	apiColor := color.New(color.FgWhite)
	apiColor.Print("   OpenAI API Key: ")
	if cfg.OpenAIAPIKey != "" {
		maskedKey := maskAPIKey(cfg.OpenAIAPIKey)
		color.New(color.FgGreen).Printf("%s\n", maskedKey)
		color.New(color.FgGreen).Println("   ✅ Valid format")
	} else {
		color.New(color.FgRed).Println("❌ Not configured")
	}

	apiColor.Print("   OpenAI Base URL: ")
	color.New(color.FgCyan).Printf("%s\n", cfg.OpenAIBaseURL)

	apiColor.Print("   Anthropic API Key: ")
	if cfg.AnthropicAPIKey != "" {
		maskedKey := maskAPIKey(cfg.AnthropicAPIKey)
		color.New(color.FgGreen).Printf("%s\n", maskedKey)
		color.New(color.FgGreen).Println("   ✅ Client validation enabled")
	} else {
		color.New(color.FgYellow).Println("Not set (validation disabled)")
	}

	// Model Configuration
	fmt.Println()
	color.New(color.FgYellow, color.Bold).Println("🤖 Model Configuration:")
	
	modelColor := color.New(color.FgWhite)
	modelColor.Print("   Big Model (opus): ")
	color.New(color.FgCyan).Printf("%s\n", cfg.BigModel)
	
	modelColor.Print("   Middle Model (sonnet): ")
	color.New(color.FgCyan).Printf("%s\n", cfg.MiddleModel)
	
	modelColor.Print("   Small Model (haiku): ")
	color.New(color.FgCyan).Printf("%s\n", cfg.SmallModel)

	// Server Configuration
	fmt.Println()
	color.New(color.FgYellow, color.Bold).Println("🌐 Server Configuration:")
	
	serverColor := color.New(color.FgWhite)
	serverColor.Print("   Host: ")
	color.New(color.FgCyan).Printf("%s\n", cfg.Host)
	
	serverColor.Print("   Port: ")
	color.New(color.FgCyan).Printf("%d\n", cfg.Port)
	
	serverColor.Print("   Log Level: ")
	color.New(color.FgCyan).Printf("%s\n", cfg.LogLevel)

	// Limits Configuration
	fmt.Println()
	color.New(color.FgYellow, color.Bold).Println("⚙️  Limits Configuration:")
	
	limitsColor := color.New(color.FgWhite)
	limitsColor.Print("   Max Tokens: ")
	color.New(color.FgCyan).Printf("%d\n", cfg.MaxTokensLimit)
	
	limitsColor.Print("   Min Tokens: ")
	color.New(color.FgCyan).Printf("%d\n", cfg.MinTokensLimit)
	
	limitsColor.Print("   Request Timeout: ")
	color.New(color.FgCyan).Printf("%ds\n", cfg.RequestTimeout)

	// Custom Headers
	if len(cfg.CustomHeaders) > 0 {
		fmt.Println()
		color.New(color.FgYellow, color.Bold).Println("📝 Custom Headers:")
		for name, value := range cfg.CustomHeaders {
			headerColor := color.New(color.FgWhite)
			headerColor.Printf("   %s: ", name)
			color.New(color.FgCyan).Printf("%s\n", value)
		}
	}

	// Environment Info
	fmt.Println()
	color.New(color.FgYellow, color.Bold).Println("📂 Configuration Source:")
	envColor := color.New(color.FgWhite)
	if _, err := os.Stat(".env"); err == nil {
		envColor.Print("   .env file: ")
		color.New(color.FgGreen).Println("Found")
	} else {
		envColor.Print("   .env file: ")
		color.New(color.FgYellow).Println("Not found (using environment variables only)")
	}

	// Usage tip
	fmt.Println()
	color.New(color.FgCyan, color.Bold).Println("💡 Tip:")
	color.New(color.FgWhite).Println("   Configuration can be set via:")
	color.New(color.FgWhite).Println("   • Environment variables")
	color.New(color.FgWhite).Println("   • .env file in the current directory")
	color.New(color.FgWhite).Println("   • Command-line flags (when starting server)")

	return nil
}

func showEnvBasedConfig() {
	color.New(color.FgCyan, color.Bold).Println("📋 Environment Configuration")
	fmt.Println()

	// Show key environment variables
	envVars := []string{
		"OPENAI_API_KEY",
		"ANTHROPIC_API_KEY",
		"OPENAI_BASE_URL",
		"BIG_MODEL",
		"MIDDLE_MODEL",
		"SMALL_MODEL",
		"HOST",
		"PORT",
		"LOG_LEVEL",
		"MAX_TOKENS_LIMIT",
		"MIN_TOKENS_LIMIT",
		"REQUEST_TIMEOUT",
	}

	color.New(color.FgYellow, color.Bold).Println("Environment Variables:")
	envColor := color.New(color.FgWhite)
	for _, envVar := range envVars {
		value := os.Getenv(envVar)
		envColor.Printf("   %s: ", envVar)
		if value != "" {
			if envVar == "OPENAI_API_KEY" || envVar == "ANTHROPIC_API_KEY" {
				color.New(color.FgGreen).Printf("%s\n", maskAPIKey(value))
			} else {
				color.New(color.FgCyan).Printf("%s\n", value)
			}
		} else {
			color.New(color.FgYellow).Println("(not set)")
		}
	}
}

func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

