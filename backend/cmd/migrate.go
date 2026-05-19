package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Database migration management",
	Long:  `Manage database migrations for the load balancer enhancements`,
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Run all pending migrations",
	Long:  `Apply all pending database migrations`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Initialize database
		dbPath := "data/proxy.db"
		if err := database.InitDB(dbPath); err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		defer database.CloseDB()

		fmt.Println("Running migrations...")
		if err := database.RunMigrations(); err != nil {
			return fmt.Errorf("failed to run migrations: %w", err)
		}

		fmt.Println("\n✅ Migrations completed successfully")
		return nil
	},
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show migration status",
	Long:  `Display the status of all migrations (applied/pending)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Initialize database
		dbPath := "data/proxy.db"
		if err := database.InitDB(dbPath); err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		defer database.CloseDB()

		migrations, err := database.GetMigrationStatus()
		if err != nil {
			return fmt.Errorf("failed to get migration status: %w", err)
		}

		if len(migrations) == 0 {
			fmt.Println("No migrations found")
			return nil
		}

		// Print migration status in a table
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "VERSION\tNAME\tSTATUS\tAPPLIED AT")
		fmt.Fprintln(w, "-------\t----\t------\t----------")

		for _, m := range migrations {
			status := "Pending"
			appliedAt := "-"
			if m.AppliedAt != nil {
				status = "Applied"
				appliedAt = m.AppliedAt.Format("2006-01-02 15:04:05")
			}
			fmt.Fprintf(w, "%03d\t%s\t%s\t%s\n", m.Version, m.Name, status, appliedAt)
		}

		w.Flush()
		return nil
	},
}

var migrateRollbackCmd = &cobra.Command{
	Use:   "rollback [version]",
	Short: "Rollback a specific migration",
	Long:  `Rollback a specific migration by version number (manual process)`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var version int
		if _, err := fmt.Sscanf(args[0], "%d", &version); err != nil {
			return fmt.Errorf("invalid version number: %s", args[0])
		}

		// Initialize database
		dbPath := "data/proxy.db"
		if err := database.InitDB(dbPath); err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		defer database.CloseDB()

		if err := database.RollbackMigration(version); err != nil {
			return err
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.AddCommand(migrateUpCmd)
	migrateCmd.AddCommand(migrateStatusCmd)
	migrateCmd.AddCommand(migrateRollbackCmd)
}
