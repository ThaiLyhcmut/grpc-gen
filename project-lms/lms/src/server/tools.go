//go:build tools

// Pin the gqlgen build tool so that `go mod tidy` keeps it as a dep.
// Run `go run github.com/99designs/gqlgen generate` from this dir to
// (re)generate graph/generated and graph/model after editing the schema.

package tools

import (
	_ "github.com/99designs/gqlgen"
	_ "github.com/99designs/gqlgen/graphql/introspection"
)
