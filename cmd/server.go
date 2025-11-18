package cmd

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/handler"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
)

var (
	host        string
	port        int
	logLevel    string
	openaiURL   string
	bigModel    string
	middleModel string
	smallModel  string
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the API proxy server",
	Long: `Start the API Proxy Server

Starts the Claude-to-OpenAI API proxy server on the specified host and port.

The server will:
  • Listen for Claude API requests
  • Translate them to OpenAI API format
  • Return responses in Claude format
  • Support streaming and non-streaming modes
  • Provide health monitoring endpoints

Configuration can be provided via:
  • Environment variables (.env file)
  • Command-line flags (override environment variables)`,
	RunE: runServer,
}

func init() {
	rootCmd.AddCommand(serverCmd)

	// Server configuration flags
	serverCmd.Flags().StringVarP(&host, "host", "H", "", "Server host (default: 0.0.0.0, or from HOST env var)")
	serverCmd.Flags().IntVarP(&port, "port", "p", 10086, "Server port (default: 10086, or from PORT env var)")
	serverCmd.Flags().StringVarP(&logLevel, "log-level", "l", "", "Log level: DEBUG, INFO, WARN, ERROR (default: INFO)")
	serverCmd.Flags().StringVarP(&openaiURL, "openai-url", "u", "", "OpenAI API base URL (default: https://api.openai.com/v1)")
	serverCmd.Flags().StringVarP(&bigModel, "big-model", "b", "", "Model for opus requests (default: gpt-4o)")
	serverCmd.Flags().StringVarP(&middleModel, "middle-model", "m", "", "Model for sonnet requests (default: gpt-4o)")
	serverCmd.Flags().StringVarP(&smallModel, "small-model", "s", "", "Model for haiku requests (default: gpt-4o-mini)")
}

func runServer(cmd *cobra.Command, args []string) error {
	// Override environment variables with flag values if provided
	// Check if flags were explicitly set using cobra's Changed() method
	if cmd.Flags().Changed("host") {
		os.Setenv("HOST", host)
	}
	if cmd.Flags().Changed("port") {
		os.Setenv("PORT", fmt.Sprintf("%d", port))
	}
	if cmd.Flags().Changed("log-level") {
		os.Setenv("LOG_LEVEL", logLevel)
	}
	if cmd.Flags().Changed("openai-url") {
		os.Setenv("OPENAI_BASE_URL", openaiURL)
	}
	if cmd.Flags().Changed("big-model") {
		os.Setenv("BIG_MODEL", bigModel)
	}
	if cmd.Flags().Changed("middle-model") {
		os.Setenv("MIDDLE_MODEL", middleModel)
	}
	if cmd.Flags().Changed("small-model") {
		os.Setenv("SMALL_MODEL", smallModel)
	}

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		color.New(color.FgRed, color.Bold).Print("❌ Configuration Error: ")
		color.New(color.FgRed).Println(err)
		return err
	}

	// Set Gin mode based on log level
	if cfg.LogLevel == "DEBUG" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create Gin router
	router := gin.Default()

	// Create handler
	h := handler.NewHandler(cfg)

	// Setup routes
	router.GET("/", h.Root)
	router.GET("/health", h.HealthCheck)
	router.GET("/test-connection", h.TestConnection)

	// Protected routes
	api := router.Group("/v1")
	api.Use(h.ValidateAPIKey())
	{
		api.POST("/messages", h.CreateMessage)
		api.POST("/messages/count_tokens", h.CountTokens)
		// Multi-config support
		api.POST("/configs/:id/messages", h.CreateMessageWithConfig)
	}

	// Find available port
	actualPort := cfg.Port
	if !utils.IsPortAvailable(actualPort) {
		color.New(color.FgYellow, color.Bold).Print("⚠️  Port ")
		color.New(color.FgYellow).Printf("%d", actualPort)
		color.New(color.FgYellow).Println(" is busy, searching for available port...")
		availablePort, err := utils.FindAvailablePort(actualPort)
		if err != nil {
			color.New(color.FgRed, color.Bold).Print("❌ Failed to find available port: ")
			color.New(color.FgRed).Println(err)
			return err
		}
		actualPort = availablePort
		cfg.Port = actualPort
		color.New(color.FgGreen, color.Bold).Print("✅ Found available port: ")
		color.New(color.FgGreen).Printf("%d\n", actualPort)
	}

	// Print startup information with colors
	printStartupInfo(cfg, actualPort)

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Host, actualPort)
	color.New(color.FgCyan, color.Bold).Println("\n🚀 Server starting...")
	if err := router.Run(addr); err != nil {
		color.New(color.FgRed, color.Bold).Print("❌ Failed to start server: ")
		color.New(color.FgRed).Println(err)
		return err
	}

	return nil
}

func printStartupInfo(cfg *config.Config, actualPort int) {
	// Header
	color.New(color.FgCyan, color.Bold).Print("🚀 Claude-to-OpenAI API Proxy (Golang) ")
	color.New(color.FgWhite).Printf("v%s\n", Version)

	// Configuration section
	color.New(color.FgGreen, color.Bold).Println("✅ Configuration loaded successfully")

	// Configuration details
	fmt.Println()
	color.New(color.FgYellow, color.Bold).Println("📋 Configuration:")

	configColor := color.New(color.FgWhite)
	configColor.Printf("   OpenAI Base URL: ")
	color.New(color.FgCyan).Printf("%s\n", cfg.OpenAIBaseURL)

	configColor.Printf("   Big Model (opus): ")
	color.New(color.FgCyan).Printf("%s\n", cfg.BigModel)

	configColor.Printf("   Middle Model (sonnet): ")
	color.New(color.FgCyan).Printf("%s\n", cfg.MiddleModel)

	configColor.Printf("   Small Model (haiku): ")
	color.New(color.FgCyan).Printf("%s\n", cfg.SmallModel)

	configColor.Printf("   Max Tokens Limit: ")
	color.New(color.FgCyan).Printf("%d\n", cfg.MaxTokensLimit)

	configColor.Printf("   Request Timeout: ")
	color.New(color.FgCyan).Printf("%ds\n", cfg.RequestTimeout)

	configColor.Printf("   Server: ")
	color.New(color.FgCyan).Printf("%s:%d\n", cfg.Host, actualPort)

	if cfg.AnthropicAPIKey != "" {
		configColor.Printf("   Client API Key Validation: ")
		color.New(color.FgGreen).Println("Enabled")
	} else {
		configColor.Printf("   Client API Key Validation: ")
		color.New(color.FgYellow).Println("Disabled")
	}

	if len(cfg.CustomHeaders) > 0 {
		configColor.Printf("   Custom Headers: ")
		color.New(color.FgCyan).Printf("%d configured\n", len(cfg.CustomHeaders))
	}

	// Endpoints section
	fmt.Println()
	color.New(color.FgYellow, color.Bold).Println("🔗 Endpoints:")

	endpointColor := color.New(color.FgWhite)
	endpointColor.Print("   POST ")
	color.New(color.FgCyan).Print("/v1/messages\n")

	endpointColor.Print("   POST ")
	color.New(color.FgCyan).Print("/v1/messages/count_tokens\n")

	endpointColor.Print("   GET  ")
	color.New(color.FgCyan).Print("/health\n")

	endpointColor.Print("   GET  ")
	color.New(color.FgCyan).Print("/test-connection\n")

	endpointColor.Print("   GET  ")
	color.New(color.FgCyan).Print("/\n")

	// Usage section
	fmt.Println()
	color.New(color.FgYellow, color.Bold).Println("💡 Use with Claude Code CLI:")
	usageColor := color.New(color.FgWhite)
	if cfg.AnthropicAPIKey != "" {
		usageColor.Printf("   ANTHROPIC_BASE_URL=http://localhost:%d ANTHROPIC_API_KEY=\"your-matching-key\" claude\n", actualPort)
	} else {
		usageColor.Printf("   ANTHROPIC_BASE_URL=http://localhost:%d ANTHROPIC_API_KEY=\"any-value\" claude\n", actualPort)
	}

	// Server address
	fmt.Println()
	color.New(color.FgGreen, color.Bold).Print("📡 Listen on: ")
	color.New(color.FgCyan).Printf("http://%s:%d\n", cfg.Host, actualPort)
}
