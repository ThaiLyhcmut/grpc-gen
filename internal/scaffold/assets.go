package scaffold

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed assets/*
var assetsFS embed.FS

func copyTemplates() error {
	// Copy template files from assets
	templates := []string{
		"crud_handler.tmpl",
		"dockerfile.tmpl",
		"docker-compose.tmpl",
		"entity_handler.tmpl",
		"env.tmpl",
		"handler.tmpl",
		"main.tmpl",
	}

	for _, tmpl := range templates {
		data, err := assetsFS.ReadFile("assets/" + tmpl)
		if err != nil {
			return fmt.Errorf("failed to read template %s: %w", tmpl, err)
		}

		if err := os.WriteFile(filepath.Join("template", tmpl), data, 0644); err != nil {
			return fmt.Errorf("failed to write template %s: %w", tmpl, err)
		}
	}

	return nil
}

// copyPkgFiles copies pkg directory from assets/template/pkg to src/service/pkg
// and replaces hardcoded module paths with actual module path
func copyPkgFiles() error {
	srcPath := "assets/template/pkg"
	dstPath := filepath.Join("src", "service", "pkg")

	// Get module path from go.mod
	modulePath, err := getModulePath()
	if err != nil {
		return fmt.Errorf("failed to get module path: %w", err)
	}

	// Copy directory with path replacement
	return copyEmbedDirWithReplace(srcPath, dstPath, map[string]string{
		"thaily/proto/common": modulePath + "/proto/common",
	})
}

// getModulePath reads module path from go.mod
func getModulePath() (string, error) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}

	return "", fmt.Errorf("module path not found in go.mod")
}

// copyEmbedDirWithReplace copies embedded directory and replaces strings in files
func copyEmbedDirWithReplace(srcPath, dstPath string, replacements map[string]string) error {
	return fs.WalkDir(assetsFS, srcPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == srcPath {
			return nil
		}

		relPath, err := filepath.Rel(srcPath, path)
		if err != nil {
			return err
		}

		dstFilePath := filepath.Join(dstPath, relPath)

		if d.IsDir() {
			return os.MkdirAll(dstFilePath, 0755)
		}

		// Read file
		data, err := assetsFS.ReadFile(path)
		if err != nil {
			return err
		}

		// Replace strings in .go files
		if strings.HasSuffix(path, ".go") {
			content := string(data)
			for old, new := range replacements {
				content = strings.ReplaceAll(content, old, new)
			}
			data = []byte(content)
		}

		return os.WriteFile(dstFilePath, data, 0644)
	})
}

// copyCertsSetup copies CERTS_SETUP.md from assets
func copyCertsSetup() error {
	data, err := assetsFS.ReadFile("assets/template/CERTS_SETUP.md")
	if err != nil {
		return fmt.Errorf("failed to read CERTS_SETUP.md: %w", err)
	}

	if err := os.WriteFile("CERTS_SETUP.md", data, 0644); err != nil {
		return fmt.Errorf("failed to write CERTS_SETUP.md: %w", err)
	}

	return nil
}

// copyGenerateCertsScript copies generate-certs.sh script from assets
func copyGenerateCertsScript() error {
	data, err := assetsFS.ReadFile("assets/template/generate-certs.sh")
	if err != nil {
		return fmt.Errorf("failed to read generate-certs.sh: %w", err)
	}

	if err := os.WriteFile("generate-certs.sh", data, 0755); err != nil {
		return fmt.Errorf("failed to write generate-certs.sh: %w", err)
	}

	return nil
}

func copyGeneratorScripts() error {
	// Copy all scripts directories
	scriptDirs := []string{"types", "parser", "generator", "utils"}

	for _, dir := range scriptDirs {
		srcPath := "assets/scripts/" + dir
		dstPath := filepath.Join("scripts", dir)

		if err := copyEmbedDir(srcPath, dstPath); err != nil {
			return fmt.Errorf("failed to copy %s: %w", dir, err)
		}
	}

	// Copy main gen_skeleton.go
	data, err := assetsFS.ReadFile("assets/scripts/gen_skeleton.go")
	if err != nil {
		return fmt.Errorf("failed to read gen_skeleton.go: %w", err)
	}

	if err := os.WriteFile(filepath.Join("scripts", "gen_skeleton.go"), data, 0644); err != nil {
		return fmt.Errorf("failed to write gen_skeleton.go: %w", err)
	}

	// Create go.mod for scripts
	goModContent := `module gen_skeleton

go 1.24
`
	if err := os.WriteFile(filepath.Join("scripts", "go.mod"), []byte(goModContent), 0644); err != nil {
		return fmt.Errorf("failed to write scripts/go.mod: %w", err)
	}

	return nil
}

func copyEmbedDir(srcPath, dstPath string) error {
	return fs.WalkDir(assetsFS, srcPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory itself
		if path == srcPath {
			return nil
		}

		// Calculate relative path
		relPath, err := filepath.Rel(srcPath, path)
		if err != nil {
			return err
		}

		dstFilePath := filepath.Join(dstPath, relPath)

		if d.IsDir() {
			return os.MkdirAll(dstFilePath, 0755)
		}

		// Copy file
		data, err := assetsFS.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(dstFilePath, data, 0644)
	})
}

// Helper function to copy regular files (for non-embedded use)
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
