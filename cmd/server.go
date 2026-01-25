package cmd

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/handler"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
)

// These functions are implemented in frontend_embed.go (production) or frontend_dev.go (development)
var (
	getFrontendFS      func() (fs.FS, error)
	isFrontendEmbedded func() bool
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
	// ⚠️ 严禁随意修改！后端固定端口54988，前端固定端口54989
	serverCmd.Flags().StringVarP(&host, "host", "H", "", "Server host (default: 0.0.0.0, or from HOST env var)")
	serverCmd.Flags().IntVarP(&port, "port", "p", 54988, "Server port (default: 54988, or from PORT env var)")
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

	// Initialize database
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = filepath.Join(".", "data", "proxy.db")
	}
	if err := database.InitDB(dbPath); err != nil {
		color.New(color.FgRed, color.Bold).Print("❌ Database Error: ")
		color.New(color.FgRed).Println(err)
		return err
	}
	defer database.CloseDB()

	// Initialize encryption
	if err := database.InitEncryption(); err != nil {
		color.New(color.FgRed, color.Bold).Print("❌ Encryption Error: ")
		color.New(color.FgRed).Println(err)
		return err
	}

	// Load configuration (OPENAI_API_KEY is optional for UI-managed configs)
	cfg, err := config.LoadConfig()
	if err != nil {
		color.New(color.FgYellow, color.Bold).Print("⚠️  Configuration Warning: ")
		color.New(color.FgYellow).Println(err)
		color.New(color.FgCyan).Println("   The service will start without a default API configuration.")
		color.New(color.FgCyan).Println("   You can manage configurations through the web UI.")
		// Create minimal config for server startup
		cfg = &config.Config{
			Host:     "0.0.0.0",
			Port:     54988,
			LogLevel: "INFO",
		}
	}

	// Set Gin mode based on log level
	if cfg.LogLevel == "DEBUG" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create Gin router
	router := gin.Default()

	// Enable CORS - 自定义中间件
	router.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// 检查是否允许该来源
		allowed := false
		if origin == "http://localhost:54989" || origin == "http://127.0.0.1:54989" {
			allowed = true
		} else if strings.HasPrefix(origin, "https://") && strings.HasSuffix(origin, ".zhaixingren.cn") {
			allowed = true
		}

		// 调试日志
		if origin != "" {
			fmt.Printf("[CORS] Origin: %s, Allowed: %v\n", origin, allowed)
		}

		if allowed {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, x-api-key")
			c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Writer.Header().Set("Access-Control-Expose-Headers", "Content-Length")
			fmt.Printf("[CORS] Headers set for origin: %s\n", origin)
		}

		// 处理预检请求
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	})

	// Create handler
	h := handler.NewHandler(cfg)

	// Setup routes
	router.GET("/", h.Root)
	router.GET("/health", h.HealthCheck)
	router.GET("/test-connection", h.TestConnection)

	// Standard Claude API routes - 标准 Anthropic API 路径（/v1/messages）
	// Claude CLI 使用这些标准路径，不强制验证
	v1 := router.Group("/v1")
	{
		v1.POST("/messages", h.CreateMessage)
		v1.POST("/messages/count_tokens", h.CountTokens)

		// 兼容端点
		v1.GET("/me", h.GetMe)
		v1.GET("/models", h.GetModels)
		v1.GET("/organizations/:org_id/usage", h.GetOrganizationUsage)

		// Admin API endpoints - Claude CLI 实际使用 /v1/admin/me
		admin := v1.Group("/admin")
		{
			admin.GET("/me", h.GetMe)
			admin.GET("/models", h.GetModels)
			admin.GET("/organizations/:org_id/usage", h.GetOrganizationUsage)
		}
	}

	// Per-config Claude API endpoints (每个配置独立的路径)
	// 使用路径: /proxy/:id/v1/messages
	proxyGroup := router.Group("/proxy/:id/v1")
	{
		proxyGroup.POST("/messages", h.CreateMessageWithConfig)
		proxyGroup.POST("/messages/count_tokens", h.CountTokens)

		// 兼容端点
		proxyGroup.GET("/me", h.GetMe)
		proxyGroup.GET("/models", h.GetModels)
		proxyGroup.GET("/organizations/:org_id/usage", h.GetOrganizationUsage)

		// Admin API endpoints - Claude CLI 实际使用 /proxy/:id/v1/admin/me
		proxyAdmin := proxyGroup.Group("/admin")
		{
			proxyAdmin.GET("/me", h.GetMe)
			proxyAdmin.GET("/models", h.GetModels)
			proxyAdmin.GET("/organizations/:org_id/usage", h.GetOrganizationUsage)
		}
	}

	// Auth API (no auth required)
	authAPI := router.Group("/api/auth")
	{
		authAPI.OPTIONS("/initialized", func(c *gin.Context) { c.Status(http.StatusOK) })
		authAPI.OPTIONS("/initialize", func(c *gin.Context) { c.Status(http.StatusOK) })
		authAPI.OPTIONS("/login", func(c *gin.Context) { c.Status(http.StatusOK) })

		authAPI.GET("/initialized", h.CheckInitialized)
		authAPI.POST("/initialize", h.InitializeSystem)
		authAPI.POST("/login", h.Login)
	}

	// Config management API (requires auth)
	configAPI := router.Group("/api")
	configAPI.Use(handler.AuthMiddleware())
	{
		// Config CRUD
		configAPI.GET("/configs", h.GetAllConfigs)
		configAPI.GET("/configs/:id", h.GetConfig)
		configAPI.POST("/configs", h.CreateConfig)
		configAPI.PUT("/configs/:id", h.UpdateConfig)
		configAPI.DELETE("/configs/:id", h.DeleteConfig)

		// Stats and logs
		configAPI.GET("/configs/:id/stats", h.GetConfigStats)
		configAPI.GET("/configs/:id/client-stats", h.GetClientStats)
		configAPI.GET("/configs/:id/logs", h.GetConfigLogs)
		configAPI.DELETE("/configs/:id/logs", h.DeleteConfigLogs)
		configAPI.GET("/configs/:id/logs/:log_id", h.GetLogDetail)
		configAPI.GET("/configs/:id/models", h.GetAvailableModels)

		// Global model history endpoint (for autocomplete in model selector)
		configAPI.GET("/models/history", h.GetHistoricalModels)

		// Test endpoint
		configAPI.POST("/configs/:id/renew-key", h.RenewConfigAPIKey)
		configAPI.POST("/configs/:id/test", h.TestConfig)

		// Load Balancer CRUD
		configAPI.GET("/load-balancers", handler.GetAllLoadBalancers)
		configAPI.GET("/load-balancers/:id", handler.GetLoadBalancer)
		configAPI.POST("/load-balancers", handler.CreateLoadBalancer)
		configAPI.PUT("/load-balancers/:id", handler.UpdateLoadBalancer)
		configAPI.DELETE("/load-balancers/:id", handler.DeleteLoadBalancer)
		configAPI.POST("/load-balancers/:id/renew-key", handler.RenewLoadBalancerKey)
		configAPI.POST("/load-balancers/:id/test", handler.TestLoadBalancer)
		configAPI.GET("/load-balancers/:id/stats", handler.GetLoadBalancerStats)

		// Load Balancer Enhanced Features
		configAPI.GET("/load-balancers/:id/health", handler.GetLoadBalancerHealthStatus)
		configAPI.POST("/load-balancers/:id/health/trigger", handler.TriggerHealthCheck)
		configAPI.GET("/load-balancers/:id/circuit-breakers", handler.GetLoadBalancerCircuitBreakers)
		configAPI.POST("/load-balancers/:id/circuit-breakers/:config_id/reset", handler.ResetCircuitBreaker)
		configAPI.GET("/load-balancers/:id/enhanced-stats", handler.GetLoadBalancerEnhancedStats)
		configAPI.GET("/load-balancers/:id/metrics/realtime", handler.GetLoadBalancerRealTimeMetrics)
		configAPI.GET("/load-balancers/:id/request-logs", handler.GetLoadBalancerRequestLogs)
		configAPI.GET("/load-balancers/:id/alerts", handler.GetLoadBalancerAlerts)
		configAPI.POST("/load-balancers/:id/alerts/:alert_id/acknowledge", handler.AcknowledgeAlert)
	}

	// Serve UI static files (embedded or from filesystem)
	frontendFS, err := getFrontendFS()
	if err == nil {
		ServeFrontend(router, frontendFS, isFrontendEmbedded())
		if isFrontendEmbedded() {
			color.New(color.FgCyan).Println("📦 Using embedded frontend files")
		} else {
			color.New(color.FgCyan).Println("📁 Using filesystem frontend files (development mode)")
		}
	} else {
		color.New(color.FgYellow).Printf("⚠️  Frontend files not available: %v\n", err)
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
	color.New(color.FgCyan, color.Bold).Print("🚀 Use ClaudeCode CLI With OpenAI API ")
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
