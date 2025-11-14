package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

const Version = "v0.2.0"

var rootCmd = &cobra.Command{
	Use:   "grpc-gen",
	Short: "A CLI tool to generate gRPC microservices with CRUD operations",
	Long: `grpc-gen helps you quickly scaffold gRPC microservices with:
- Full CRUD operations (Create, Read, Update, Delete, List)
- Database integration with MySQL
- Automatic code generation from proto files
- TLS/mTLS support with certificate management
- Function tracing and file logging
- Docker and docker-compose configuration
- Environment-based configuration

Example usage:
  grpc-gen init my-project
  grpc-gen add-service user 50051`,
	Version: Version,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(addServiceCmd)
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of grpc-gen",
	Long:  `All software has versions. This is grpc-gen's`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("grpc-gen %s\n", Version)
		fmt.Println("\nFeatures:")
		fmt.Println("  ✓ TLS/mTLS support")
		fmt.Println("  ✓ Function tracing & logging")
		fmt.Println("  ✓ Docker & docker-compose")
		fmt.Println("  ✓ Full CRUD operations")
		fmt.Println("  ✓ Database connection pooling")
		fmt.Println("  ✓ Advanced filtering & pagination")
	},
}
