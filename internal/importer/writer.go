package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteKdefFiles writes the import result to .kdef files in the output directory.
func WriteKdefFiles(result ImportResult, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	// Write deployments — one file per deployment
	for _, dep := range result.Deployments {
		name := extractBlockName(dep)
		path := filepath.Join(outputDir, name+".kdef")
		if err := os.WriteFile(path, []byte(dep), 0644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		fmt.Printf("wrote %s\n", path)
	}

	// Write cronjobs — all in one file
	if len(result.CronJobs) > 0 {
		content := strings.Join(result.CronJobs, "\n")
		path := filepath.Join(outputDir, "cronjobs.kdef")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write cronjobs.kdef: %w", err)
		}
		fmt.Printf("wrote %s\n", path)
	}

	// Write configmaps — all in one file
	if len(result.ConfigMaps) > 0 {
		content := strings.Join(result.ConfigMaps, "\n")
		path := filepath.Join(outputDir, "configmaps.kdef")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write configmaps.kdef: %w", err)
		}
		fmt.Printf("wrote %s\n", path)
	}

	// Write PVCs — all in one file
	if len(result.PersistentVolumeClaims) > 0 {
		content := strings.Join(result.PersistentVolumeClaims, "\n")
		path := filepath.Join(outputDir, "pvcs.kdef")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write pvcs.kdef: %w", err)
		}
		fmt.Printf("wrote %s\n", path)
	}

	return nil
}

// PrintKdef prints the import result to stdout.
func PrintKdef(result ImportResult) {
	first := true
	for _, block := range result.Deployments {
		if !first {
			fmt.Println()
		}
		first = false
		fmt.Print(block)
	}
	for _, block := range result.CronJobs {
		fmt.Println()
		fmt.Print(block)
	}
	for _, block := range result.ConfigMaps {
		fmt.Println()
		fmt.Print(block)
	}
	for _, block := range result.PersistentVolumeClaims {
		fmt.Println()
		fmt.Print(block)
	}
}

// extractBlockName extracts the name from a kdef block string like `app "my-name" {`.
func extractBlockName(block string) string {
	// Find the quoted name after the block type
	start := strings.Index(block, `"`)
	if start == -1 {
		return "unknown"
	}
	end := strings.Index(block[start+1:], `"`)
	if end == -1 {
		return "unknown"
	}
	return block[start+1 : start+1+end]
}
