package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/thailyhcmut/grpc-gen/internal/scaffold"
)

var initCmd = &cobra.Command{
	Use:   "init [project-name]",
	Short: "Initialize a new gRPC project",
	Long: `Initialize a new gRPC project with all necessary scaffolding:
- Project structure
- Proto definitions
- Common utilities (logger, database)
- Makefile for building
- Docker configuration`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := args[0]

		// Get module path from flag or use default
		modulePath, _ := cmd.Flags().GetString("module")
		if modulePath == "" {
			modulePath = projectName
		}

		// Create project directory
		if err := os.MkdirAll(projectName, 0755); err != nil {
			return fmt.Errorf("failed to create project directory: %w", err)
		}

		// Change to project directory
		if err := os.Chdir(projectName); err != nil {
			return fmt.Errorf("failed to change to project directory: %w", err)
		}

		absPath, _ := filepath.Abs(".")
		fmt.Printf("📦 Creating project in: %s\n\n", absPath)

		// Scaffold the project
		if err := scaffold.CreateProject(projectName, modulePath); err != nil {
			return fmt.Errorf("failed to scaffold project: %w", err)
		}

		fmt.Printf("\n✅ Project created successfully!\n\n")
		fmt.Println("Next steps:")
		fmt.Printf("  cd %s\n", projectName)
		fmt.Println("  grpc-gen add-service user 50051")
		fmt.Println("  ./generate-certs.sh user          # Generate TLS certificates")
		fmt.Println("  make gen-user")
		fmt.Println("  go build ./src/service/user")
		fmt.Println()

		return nil
	},
}

func init() {
	initCmd.Flags().StringP("module", "m", "", "Go module path (default: project-name)")
}
