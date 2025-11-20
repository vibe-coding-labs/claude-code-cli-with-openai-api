package cmd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/fatih/color"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/handler"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
)

var (
	uiHost     string
	uiPort     int
	uiLogLevel string
)

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Start the API proxy server with Web UI",
	Long: `Start the API Proxy Server with Web UI

Starts the Claude-to-OpenAI API proxy server with a web-based management interface.
The server includes:
  • Configuration management UI
  • API documentation
  • Multiple configuration support
  • Configuration testing

Configuration can be provided via:
  • Environment variables (.env file)
  • Command-line flags (override environment variables)`,
	RunE: runUI,
}

func init() {
	rootCmd.AddCommand(uiCmd)

	// UI server configuration flags
	uiCmd.Flags().StringVarP(&uiHost, "host", "H", "", "Server host (default: 0.0.0.0, or from HOST env var)")
	uiCmd.Flags().IntVarP(&uiPort, "port", "p", 10086, "Server port (default: 10086, or from PORT env var)")
	uiCmd.Flags().StringVarP(&uiLogLevel, "log-level", "l", "", "Log level: DEBUG, INFO, WARN, ERROR (default: INFO)")
}

func runUI(cmd *cobra.Command, args []string) error {
	// Override environment variables with flag values if provided
	// Check if flags were explicitly set using cobra's Changed() method
	if cmd.Flags().Changed("host") {
		os.Setenv("HOST", uiHost)
	}
	if cmd.Flags().Changed("port") {
		os.Setenv("PORT", fmt.Sprintf("%d", uiPort))
	}
	if cmd.Flags().Changed("log-level") {
		os.Setenv("LOG_LEVEL", uiLogLevel)
	}

	// Load default configuration (not required for UI mode)
	// In UI mode, we can start without a default config since users can create multiple configs
	cfg := &config.Config{
		Host:            getEnvOrDefault("HOST", "0.0.0.0"),
		Port:            getEnvAsInt("PORT", 10086),
		LogLevel:        getEnvOrDefault("LOG_LEVEL", "INFO"),
		OpenAIAPIKey:    os.Getenv("OPENAI_API_KEY"), // Optional in UI mode
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		OpenAIBaseURL:   getEnvOrDefault("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		BigModel:        getEnvOrDefault("BIG_MODEL", "gpt-4o"),
		MiddleModel:     getEnvOrDefault("MIDDLE_MODEL", "gpt-4o"),
		SmallModel:      getEnvOrDefault("SMALL_MODEL", "gpt-4o-mini"),
		MaxTokensLimit:  getEnvAsInt("MAX_TOKENS_LIMIT", 4096),
		MinTokensLimit:  getEnvAsInt("MIN_TOKENS_LIMIT", 100),
		RequestTimeout:  getEnvAsInt("REQUEST_TIMEOUT", 90),
		CustomHeaders:   make(map[string]string),
	}

	// Set Gin mode based on log level
	if cfg.LogLevel == "DEBUG" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create Gin router
	router := gin.Default()

	// Enable CORS
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "x-api-key"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	// Create handler
	h := handler.NewHandler(cfg)

	// Setup API routes
	router.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/ui/")
	})

	router.GET("/health", h.HealthCheck)
	router.GET("/test-connection", h.TestConnection)

	// Claude API compatible routes
	api := router.Group("/v1")
	api.Use(h.ValidateAPIKey())
	{
		api.POST("/messages", h.CreateMessage)
		api.POST("/messages/count_tokens", h.CountTokens)
		// Multi-config support
		api.POST("/configs/:id/messages", h.CreateMessageWithConfig)
	}

	// Configuration management API
	configHandler := handler.NewConfigHandler()
	apiConfig := router.Group("/api/configs")
	{
		apiConfig.GET("", configHandler.ListConfigs)
		apiConfig.GET("/:id", configHandler.GetConfig)
		apiConfig.POST("", configHandler.CreateConfig)
		apiConfig.PUT("/:id", configHandler.UpdateConfig)
		apiConfig.DELETE("/:id", configHandler.DeleteConfig)
		apiConfig.POST("/:id/test", configHandler.TestConfig)
		apiConfig.POST("/:id/set-default", configHandler.SetDefaultConfig)
		apiConfig.GET("/:id/claude-config", configHandler.GetClaudeConfig)
	}

	// API documentation
	router.GET("/api/docs", configHandler.GetAPIDocs)

	// Serve static files from frontend build directory
	frontendBuildPath := filepath.Join(".", "frontend", "build")
	if _, err := os.Stat(frontendBuildPath); os.IsNotExist(err) {
		color.New(color.FgYellow, color.Bold).Println("⚠️  Frontend build not found. Building frontend...")
		// Try to build frontend
		if err := buildFrontend(); err != nil {
			color.New(color.FgRed, color.Bold).Print("❌ Failed to build frontend: ")
			color.New(color.FgRed).Println(err)
			color.New(color.FgYellow).Println("💡 Please build the frontend manually: cd frontend && npm run build")
			return fmt.Errorf("frontend build not found")
		}
	}

	// Serve UI files - handle static files and React Router
	router.GET("/ui", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/ui/")
	})

	// Custom handler for all /ui/* routes
	router.GET("/ui/*filepath", func(c *gin.Context) {
		path := c.Param("filepath")

		// Handle root path
		if path == "/" || path == "" {
			c.File(filepath.Join(frontendBuildPath, "index.html"))
			return
		}

		// Remove leading slash
		if len(path) > 0 && path[0] == '/' {
			path = path[1:]
		}

		// Handle static files: /ui/static/* -> frontend/build/static/*
		if len(path) > 7 && path[:7] == "static/" {
			staticPath := filepath.Join(frontendBuildPath, path)
			if info, err := os.Stat(staticPath); err == nil && !info.IsDir() {
				c.File(staticPath)
				return
			}
			// Static file not found
			c.Status(http.StatusNotFound)
			return
		}

		// Handle other files in build root (favicon.ico, manifest.json, etc.)
		fullPath := filepath.Join(frontendBuildPath, path)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			c.File(fullPath)
			return
		}

		// For all other paths (React Router routes), serve index.html
		c.File(filepath.Join(frontendBuildPath, "index.html"))
	})

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
	printUIStartupInfo(cfg, actualPort)

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Host, actualPort)
	color.New(color.FgCyan, color.Bold).Println("\n🚀 Server starting with Web UI...")
	if err := router.Run(addr); err != nil {
		color.New(color.FgRed, color.Bold).Print("❌ Failed to start server: ")
		color.New(color.FgRed).Println(err)
		return err
	}

	return nil
}

func buildFrontend() error {
	// This would require executing npm commands
	// For now, we'll just return an error and let the user build manually
	return fmt.Errorf("automatic frontend build not implemented")
}

func printUIStartupInfo(cfg *config.Config, actualPort int) {
	// Header
	color.New(color.FgCyan, color.Bold).Print("🚀 Use ClaudeCode CLI With OpenAI API ")
	color.New(color.FgWhite).Printf("v%s\n", Version)

	// Web UI section
	fmt.Println()
	color.New(color.FgGreen, color.Bold).Println("✅ Web UI enabled")

	// Server address
	fmt.Println()
	color.New(color.FgGreen, color.Bold).Print("📡 Server: ")
	color.New(color.FgCyan).Printf("http://%s:%d\n", cfg.Host, actualPort)

	color.New(color.FgGreen, color.Bold).Print("🌐 Web UI: ")
	color.New(color.FgCyan).Printf("http://localhost:%d/ui\n", actualPort)

	// API endpoints
	fmt.Println()
	color.New(color.FgYellow, color.Bold).Println("🔗 API Endpoints:")
	endpointColor := color.New(color.FgWhite)
	endpointColor.Print("   POST ")
	color.New(color.FgCyan).Print("/v1/messages\n")
	endpointColor.Print("   POST ")
	color.New(color.FgCyan).Print("/v1/configs/:id/messages\n")
	endpointColor.Print("   GET  ")
	color.New(color.FgCyan).Print("/api/configs\n")
	endpointColor.Print("   GET  ")
	color.New(color.FgCyan).Print("/api/docs\n")
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
