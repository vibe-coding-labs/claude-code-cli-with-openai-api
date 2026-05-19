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
	"golang.org/x/term"
)

var resetPasswordCmd = &cobra.Command{
	Use:   "reset-password",
	Short: "Reset system password",
	RunE:  runResetPassword,
}

func init() {
	rootCmd.AddCommand(resetPasswordCmd)
}

func runResetPassword(cmd *cobra.Command, args []string) error {
	dbPath := resolveDBPath()

	if err := database.InitDB(dbPath); err != nil {
		color.Red("Error: %v", err)
		return err
	}
	defer database.CloseDB()

	reader := bufio.NewReader(os.Stdin)

	// Prompt for username
	fmt.Print("Username: ")
	username, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}
	username = strings.TrimSpace(username)
	if len(username) < 3 {
		color.Red("Username must be at least 3 characters")
		return fmt.Errorf("username too short")
	}

	// Prompt for password
	fmt.Print("Password: ")
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}
	fmt.Println()
	if len(passwordBytes) < 6 {
		color.Red("Password must be at least 6 characters")
		return fmt.Errorf("password too short")
	}

	// Confirm password
	fmt.Print("Confirm password: ")
	confirmBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}
	fmt.Println()

	if string(passwordBytes) != string(confirmBytes) {
		color.Red("Passwords do not match")
		return fmt.Errorf("passwords do not match")
	}

	// Delete all existing users
	if err := database.DeleteAllUsers(); err != nil {
		color.Red("Failed to reset: %v", err)
		return err
	}

	// Create new user
	if err := database.CreateUser(username, string(passwordBytes)); err != nil {
		color.Red("Failed to create user: %v", err)
		return err
	}

	color.Green("Password reset successful.")
	return nil
}

func resolveDBPath() string {
	dbPath := os.Getenv("DB_PATH")
	if dbPath != "" {
		return dbPath
	}

	exePath, err := os.Executable()
	if err == nil {
		realPath, _ := filepath.EvalSymlinks(exePath)
		if realPath == "" {
			realPath = exePath
		}
		candidate := filepath.Join(filepath.Dir(realPath), "data", "proxy.db")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return filepath.Join(".", "data", "proxy.db")
}
