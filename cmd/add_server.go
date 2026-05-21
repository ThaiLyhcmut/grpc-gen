package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/thailyhcmut/grpc-gen/internal/scaffold"
)

var addServerCmd = &cobra.Command{
	Use:   "add-server [port]",
	Short: "Add a GraphQL server to the project",
	Long: `Add a GraphQL server that connects to all gRPC services:
- Creates GraphQL schema types from proto files
- Creates gRPC clients for each service
- Creates convert functions (proto <-> graphql)
- Creates controller base
- Creates router, config, main.go
- Creates gqlgen.yml configuration

Example:
  grpc-gen add-server 8080`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		portStr := args[0]

		// Validate port
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1024 || port > 65535 {
			return fmt.Errorf("invalid port number: %s (must be between 1024-65535)", portStr)
		}

		fmt.Printf("🚀 Adding GraphQL server on port %d\n\n", port)

		// Add server to project
		if err := scaffold.AddServer(port); err != nil {
			return fmt.Errorf("failed to add server: %w", err)
		}

		fmt.Printf("\n✅ GraphQL server added successfully!\n\n")
		fmt.Println("Next steps:")
		fmt.Println("  1. Edit graph/schema/query.graphqls to define your queries")
		fmt.Println("  2. Edit graph/schema/mutation.graphqls to define your mutations")
		fmt.Println("  3. Run: cd src/server && go run github.com/99designs/gqlgen generate")
		fmt.Println("  4. Implement resolvers in graph/resolver/")
		fmt.Println("  5. Run: go build -o server . && ./server")
		fmt.Println()

		return nil
	},
}

func init() {
	rootCmd.AddCommand(addServerCmd)
}
