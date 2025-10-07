package scaffold

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed assets/*
var assetsFS embed.FS

func copyTemplates() error {
	// Copy template files from assets
	templates := []string{
		"crud_handler.tmpl",
		"dockerfile.tmpl",
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
