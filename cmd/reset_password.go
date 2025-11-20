package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

var resetPasswordCmd = &cobra.Command{
	Use:   "reset-password",
	Short: "Reset system password by deleting all users",
	Long: `Reset System Password

This command will delete all users from the database, allowing you to re-initialize 
the system with a new username and password.

WARNING: This action cannot be undone. You will need to log in again through the 
web interface after running this command.`,
	RunE: runResetPassword,
}

func init() {
	rootCmd.AddCommand(resetPasswordCmd)
}

func runResetPassword(cmd *cobra.Command, args []string) error {
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

	// Check if users exist
	hasUser, err := database.HasUser()
	if err != nil {
		color.New(color.FgRed, color.Bold).Print("❌ Error checking users: ")
		color.New(color.FgRed).Println(err)
		return err
	}

	if !hasUser {
		color.New(color.FgYellow, color.Bold).Println("⚠️  No users found in the database.")
		color.New(color.FgCyan).Println("The system is not initialized yet. You can set up a new user through the web interface.")
		return nil
	}

	// Confirm action
	color.New(color.FgYellow, color.Bold).Println("\n⚠️  WARNING: This will delete all users!")
	color.New(color.FgWhite).Println("You will need to re-initialize the system through the web interface.")
	color.New(color.FgWhite).Print("\nAre you sure you want to continue? (yes/no): ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "yes" && response != "y" {
		color.New(color.FgCyan).Println("\n✅ Password reset cancelled.")
		return nil
	}

	// Delete all users
	if err := database.DeleteAllUsers(); err != nil {
		color.New(color.FgRed, color.Bold).Print("\n❌ Failed to reset password: ")
		color.New(color.FgRed).Println(err)
		return err
	}

	// Success message
	color.New(color.FgGreen, color.Bold).Println("\n✅ Password reset successful!")
	color.New(color.FgCyan, color.Bold).Println("\n📋 Next steps:")
	color.New(color.FgWhite).Println("   1. Start the server if not already running:")
	color.New(color.FgCyan).Println("      ./claude-code-cli-with-openai-api server")
	color.New(color.FgWhite).Println("   2. Open the web interface in your browser:")
	color.New(color.FgCyan).Println("      http://localhost:8083/ui/")
	color.New(color.FgWhite).Println("   3. You will be redirected to the initialization page")
	color.New(color.FgWhite).Println("   4. Create a new username and password")

	return nil
}
