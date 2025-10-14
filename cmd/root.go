package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "grpc-gen",
	Short: "A CLI tool to generate gRPC microservices with CRUD operations",
	Long: `grpc-gen helps you quickly scaffold gRPC microservices with:
- Full CRUD operations (Create, Read, Update, Delete, List)
- Database integration with MySQL
- Automatic code generation from proto files
- Logger support
- Docker configuration

Example usage:
  grpc-gen init my-project
  grpc-gen add-service user 50051`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(addServiceCmd)
}
