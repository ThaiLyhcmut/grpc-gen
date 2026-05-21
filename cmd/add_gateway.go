package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/thailyhcmut/grpc-gen/internal/scaffold/gateway"
)

var addGatewayCmd = &cobra.Command{
	Use:   "add-gateway [port]",
	Short: "Generate GraphQL gateway code from proto + gateway-config.yaml",
	Long: `Reads proto/* and gateway-config.yaml (relation declarations) and
generates src/server/graph/schema/*.graphqls plus supporting Go code.

Phase 1+2 (current): schema files + gqlgen.yml + resolver stub so you can review them.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port := 8080
		if len(args) == 1 {
			p, err := strconv.Atoi(args[0])
			if err != nil || p < 1024 || p > 65535 {
				return fmt.Errorf("invalid port %q", args[0])
			}
			port = p
		}

		// Sanity: must be in a grpc-gen project (go.mod present).
		if _, err := os.Stat("go.mod"); os.IsNotExist(err) {
			return fmt.Errorf("not in a grpc-gen project directory (go.mod missing)")
		}

		protoRoot := "proto"
		protos, err := gateway.ParseAllProtos(protoRoot)
		if err != nil {
			return fmt.Errorf("parse protos: %w", err)
		}
		if len(protos) == 0 {
			return fmt.Errorf("no service protos found under %s/", protoRoot)
		}
		fmt.Printf("📦 Parsed %d services:\n", len(protos))
		for _, p := range protos {
			entityCount := 0
			for _, m := range p.Messages {
				if gateway.IsEntityMessage(m.Name) {
					entityCount++
				}
			}
			fmt.Printf("   %-15s  %d entities, %d enums, %d RPCs\n", p.ServiceName, entityCount, len(p.Enums), len(p.RPCs))
		}

		cfgPath := "gateway-config.yaml"
		cfg, err := gateway.LoadConfig(cfgPath)
		if err != nil {
			return fmt.Errorf("load %s: %w", cfgPath, err)
		}
		if len(cfg.Entities) == 0 {
			fmt.Printf("\n⚠️  %s missing or empty — schemas will not contain relation fields.\n", cfgPath)
			fmt.Printf("    Create one to declare cross-service relations.\n")
		} else {
			if err := gateway.ValidateConfig(cfg, protos); err != nil {
				return fmt.Errorf("validate config: %w", err)
			}
			// Mark fields guarded by field_auth/field_policy as ModelNullable
			// so schema gen drops their `!` and converter gen pointer-wraps.
			gateway.ApplyFieldDirectives(cfg, protos)
			fmt.Printf("\n📋 Loaded %s: %d entities with relations\n", cfgPath, len(cfg.Entities))
		}

		outDir := filepath.Join("src", "server", "graph", "schema")
		if err := gateway.GenerateSchemas(gateway.SchemaGenInput{
			OutDir: outDir,
			Protos: protos,
			Config: cfg,
		}); err != nil {
			return fmt.Errorf("gen schemas: %w", err)
		}
		fmt.Printf("\n✅ Generated GraphQL schemas in %s/\n", outDir)
		fmt.Printf("   _common.graphqls\n")
		for _, p := range protos {
			fmt.Printf("   %s.graphqls\n", p.ServiceName)
		}

		// Phase 2: gqlgen wiring.
		modulePath, err := readModulePath("go.mod")
		if err != nil {
			return fmt.Errorf("read go.mod: %w", err)
		}
		serverDir := filepath.Join("src", "server")
		if err := gateway.GenerateGqlgenConfig(gateway.GqlgenGenInput{
			ServerDir:  serverDir,
			ModulePath: modulePath,
		}); err != nil {
			return fmt.Errorf("gen gqlgen.yml: %w", err)
		}
		fmt.Printf("\n✅ Generated gqlgen wiring in %s/:\n", serverDir)
		fmt.Printf("   gqlgen.yml\n")
		fmt.Printf("   tools.go (pins gqlgen as dep)\n")
		fmt.Printf("   graph/resolver/resolver.go (Resolver struct stub)\n")
		fmt.Printf("   graph/{model,generated}/ (empty dirs for gqlgen output)\n")
		// Phase 2b: cache package (must exist before client wrappers import it).
		if err := gateway.GenerateCache(gateway.CacheGenInput{
			ServerDir:  serverDir,
			ModulePath: modulePath,
		}); err != nil {
			return fmt.Errorf("gen cache: %w", err)
		}
		fmt.Printf("\n✅ Generated cache package in %s/graph/cache/:\n", serverDir)
		fmt.Printf("   cache.go (Cache interface) + redis.go + noop.go + keys.go\n")

		// Phase 2c: auth + policy + directive runners. Done EARLY (before
		// client gen) because the client wrap reads auth.Viewer for per-
		// viewer cache keys, and gqlgen's later "go build" of generated
		// code needs the runner package to already compile.
		if err := gateway.GenerateDirectives(gateway.DirectiveGenInput{
			ServerDir:  serverDir,
			ModulePath: modulePath,
		}); err != nil {
			return fmt.Errorf("gen directives: %w", err)
		}
		fmt.Printf("\n✅ Generated auth/policy scaffold:\n")
		fmt.Printf("   graph/auth/viewer.go        (Viewer + ctx helpers)\n")
		fmt.Printf("   graph/policy/enforcer.go    (Enforcer interface + DenyAll default)\n")
		fmt.Printf("   graph/directive/directive.go (@auth + @policy runners)\n")

		// Phase 3: gRPC client wrappers.
		if err := gateway.GenerateClients(gateway.ClientGenInput{
			ServerDir:  serverDir,
			ModulePath: modulePath,
			Protos:     protos,
		}); err != nil {
			return fmt.Errorf("gen clients: %w", err)
		}
		fmt.Printf("\n✅ Generated gRPC client wrappers in %s/graph/client/:\n", serverDir)
		fmt.Printf("   clients.go (aggregator + Endpoints config)\n")
		for _, p := range protos {
			fmt.Printf("   %s.go\n", p.ServiceName)
		}

		// Phase 3b: dynamic business-rule engine (depends on the client pkg).
		if err := gateway.GenerateDomain(gateway.DomainGenInput{
			ServerDir:  serverDir,
			ModulePath: modulePath,
			Protos:     protos,
		}); err != nil {
			return fmt.Errorf("gen domain: %w", err)
		}
		fmt.Printf("\n✅ Generated business-rule engine in %s/graph/domain/:\n", serverDir)
		fmt.Printf("   store.go + engine.go + interceptor.go + registry.go\n")
		fmt.Printf("   (rules live in MongoDB — see graph/domain for the schema)\n")

		// Phase 4: DataLoaders per entity.
		if err := gateway.GenerateDataLoaders(gateway.DataLoaderGenInput{
			ServerDir:  serverDir,
			ModulePath: modulePath,
			Protos:     protos,
		}); err != nil {
			return fmt.Errorf("gen dataloaders: %w", err)
		}
		fmt.Printf("\n✅ Generated DataLoaders in %s/graph/dataloader/:\n", serverDir)
		fmt.Printf("   loaders.go (aggregator + per-request context)\n")
		entityCount := 0
		for _, p := range protos {
			for _, m := range p.Messages {
				if gateway.IsEntityMessage(m.Name) {
					entityCount++
				}
			}
		}
		fmt.Printf("   %d per-entity loader files\n", entityCount)

		// Phase 5: proto ↔ model converters.
		if err := gateway.GenerateConverters(gateway.ConvertGenInput{
			ServerDir:  serverDir,
			ModulePath: modulePath,
			Protos:     protos,
		}); err != nil {
			return fmt.Errorf("gen converters: %w", err)
		}
		fmt.Printf("\n✅ Generated converters in %s/graph/convert/:\n", serverDir)
		fmt.Printf("   common.go (SearchInput / FilterCriteria)\n")
		for _, p := range protos {
			fmt.Printf("   %s.go         (PBTo<Entity> + enum maps)\n", p.ServiceName)
			fmt.Printf("   %s_input.go   (Create/Update input → PB)\n", p.ServiceName)
		}

		// Phase 5b: input converters (Create/Update inputs).
		if err := gateway.GenerateInputConverters(gateway.ConvertGenInput{
			ServerDir:  serverDir,
			ModulePath: modulePath,
			Protos:     protos,
		}); err != nil {
			return fmt.Errorf("gen input converters: %w", err)
		}

		// Run gqlgen now — it needs the schema + minimal resolver.go that
		// Phase 2 wrote. After this, generated.go has the entity Resolver
		// interfaces that Phase 6 will reference.
		fmt.Printf("\n🛠  Running gqlgen generate (this populates graph/generated + model)...\n")
		if err := runShell(".", "go", "mod", "tidy"); err != nil {
			return fmt.Errorf("go mod tidy: %w", err)
		}
		if err := runShell(serverDir, "go", "run", "github.com/99designs/gqlgen", "generate"); err != nil {
			return fmt.Errorf("gqlgen generate: %w", err)
		}

		// Phase 6: resolver implementations (replaces gqlgen panic stubs).
		if err := gateway.GenerateResolvers(gateway.ResolverGenInput{
			ServerDir:  serverDir,
			ModulePath: modulePath,
			Protos:     protos,
			Config:     cfg,
		}); err != nil {
			return fmt.Errorf("gen resolvers: %w", err)
		}
		fmt.Printf("\n✅ Generated resolver implementations in %s/graph/resolver/:\n", serverDir)
		fmt.Printf("   (resolver.go = Phase 2; common.resolvers.go = gqlgen)\n")
		for _, p := range protos {
			fmt.Printf("   %s.resolvers.go (mutations + queries + relations + sub-resolver wiring)\n", p.ServiceName)
		}

		// Phase 8: main.go (idempotent — skipped if it already exists, so
		// users can edit freely without losing their changes on rerun).
		if err := gateway.GenerateMain(gateway.MainGenInput{
			ServerDir:  serverDir,
			ModulePath: modulePath,
			Port:       port,
			Protos:     protos,
		}); err != nil {
			return fmt.Errorf("gen main.go: %w", err)
		}
		fmt.Printf("\n✅ Generated %s/main.go (skipped if pre-existing)\n", serverDir)

		// Final tidy so all new imports (golang-jwt, grpc creds, gqlgen
		// handler) resolve before the user tries `go run`.
		fmt.Printf("\n🛠  go mod tidy (final pass)...\n")
		if err := runShell(".", "go", "mod", "tidy"); err != nil {
			return fmt.Errorf("go mod tidy: %w", err)
		}

		gateway.PrintGqlgenNextSteps(serverDir)
		return nil
	},
}

// runShell invokes a command, streaming stdout/stderr through to the user
// (so they see gqlgen's output live).
func runShell(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// readModulePath extracts the module line from go.mod.
func readModulePath(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", fmt.Errorf("module path not found in %s", path)
}

func init() {
	rootCmd.AddCommand(addGatewayCmd)
}
