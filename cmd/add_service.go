package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/thailyhcmut/grpc-gen/internal/scaffold"
)

var addServiceCmd = &cobra.Command{
	Use:   "add-service [service-name] [port]",
	Short: "Add a new service to the project",
	Long: `Add a new gRPC service to your project:
- Creates proto definition
- Updates Makefile with new targets
- Ready for entity definitions

Example:
  grpc-generator add-service user 50051
  grpc-generator add-service order 50052`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		serviceName := args[0]
		portStr := args[1]

		// Validate port
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1024 || port > 65535 {
			return fmt.Errorf("invalid port number: %s (must be between 1024-65535)", portStr)
		}

		fmt.Printf("📝 Adding service: %s on port %d\n\n", serviceName, port)

		// Add service to project
		if err := scaffold.AddService(serviceName, port); err != nil {
			return fmt.Errorf("failed to add service: %w", err)
		}

		fmt.Printf("\n✅ Service added successfully!\n\n")
		fmt.Println("Next steps:")
		fmt.Printf("  1. Edit proto/%s/%s.proto to define your entities\n", serviceName, serviceName)
		fmt.Printf("  2. make gen-%s\n", serviceName)
		fmt.Printf("  3. go build ./src/service/%s\n", serviceName)
		fmt.Println()

		return nil
	},
}
