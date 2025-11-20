package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

var resetCmd = &cobra.Command{
	Use:   "reset-password",
	Short: "Reset the admin password",
	Long: `Reset the admin password by deleting all users.
	
After running this command, the system will be reinitialized and you can set a new username and password through the web UI.

WARNING: This will remove all existing users from the database.`,
	RunE: resetPassword,
}

func init() {
	rootCmd.AddCommand(resetCmd)
}

func resetPassword(cmd *cobra.Command, args []string) error {
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
		color.New(color.FgRed, color.Bold).Print("❌ Error: ")
		color.New(color.FgRed).Println(err)
		return err
	}

	if !hasUser {
		color.New(color.FgYellow, color.Bold).Println("⚠️  No users found in the database.")
		color.New(color.FgYellow).Println("   The system is not initialized yet.")
		return nil
	}

	// Confirm deletion
	color.New(color.FgYellow, color.Bold).Println("⚠️  WARNING:")
	color.New(color.FgYellow).Println("   This will delete all users and reset the system to uninitialized state.")
	fmt.Print("\nAre you sure you want to continue? (yes/no): ")

	var confirm string
	fmt.Scanln(&confirm)

	if confirm != "yes" && confirm != "y" && confirm != "YES" {
		color.New(color.FgBlue).Println("❌ Operation cancelled.")
		return nil
	}

	// Delete all users
	if err := database.DeleteAllUsers(); err != nil {
		color.New(color.FgRed, color.Bold).Print("❌ Failed to reset: ")
		color.New(color.FgRed).Println(err)
		return err
	}

	color.New(color.FgGreen, color.Bold).Println("✅ Password reset successfully!")
	color.New(color.FgBlue).Println("\n📋 Next steps:")
	color.New(color.FgBlue).Println("   1. Start the server: ./claude-with-openai-api server")
	color.New(color.FgBlue).Println("   2. Open the web UI: http://localhost:8083/ui")
	color.New(color.FgBlue).Println("   3. Set up a new username and password")

	return nil
}
