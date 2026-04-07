package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/gsid-nl/kdef/internal/generator"
	"github.com/gsid-nl/kdef/internal/parser"
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Diff rendered manifests against live cluster (requires kubectl)",
	RunE:  runDiff,
}

var (
	diffDir        string
	diffSetVars    []string
	diffValuesFile string
	diffEnv        string
)

func init() {
	diffCmd.Flags().StringVar(&diffDir, "dir", ".", "project directory containing .kdef files")
	diffCmd.Flags().StringSliceVar(&diffSetVars, "set", nil, "set variable values (key=value)")
	diffCmd.Flags().StringVar(&diffValuesFile, "values", "", "JSON values file for complex variables")
	diffCmd.Flags().StringVar(&diffEnv, "env", "", "environment name (loads environments/<env>.kdef)")
}

func runDiff(cmd *cobra.Command, args []string) error {
	overrides := parseSetFlags(diffSetVars)

	config, err := parser.LoadWithOptions(parser.LoadOptions{
		Dir:        diffDir,
		Overrides:  overrides,
		ValuesFile: diffValuesFile,
		Env:        diffEnv,
	})
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	allManifests := generator.Generate(config)
	yamlBytes, err := generator.RenderAllYAML(allManifests)
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}

	// Write to temp file for kubectl
	tmpFile, err := os.CreateTemp("", "kdef-diff-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(yamlBytes); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()

	// Run kubectl diff
	kubectlCmd := exec.Command("kubectl", "diff", "--server-side", "--force-conflicts", "-f", tmpFile.Name())
	kubectlCmd.Stdout = cmd.OutOrStdout()
	kubectlCmd.Stderr = cmd.ErrOrStderr()

	if err := kubectlCmd.Run(); err != nil {
		// kubectl diff exits with 1 when there are differences (not an error)
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil // differences found, already printed
		}
		return fmt.Errorf("kubectl diff: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "no differences")
	return nil
}
