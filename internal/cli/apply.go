package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/gsid-nl/kdef/internal/generator"
	"github.com/gsid-nl/kdef/internal/parser"
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply rendered manifests to the cluster (server-side apply)",
	Long: `Apply renders .kdef files and applies them to the cluster using
kubectl apply --server-side. This is the recommended way to deploy
kdef-managed resources.

Use --dry-run to preview what would be applied without making changes.`,
	RunE: runApply,
}

var (
	applyDir        string
	applySetVars    []string
	applyValuesFile string
	applyEnv        string
	applyVarsFrom   []string
	applyDryRun     bool
)

func init() {
	applyCmd.Flags().StringVar(&applyDir, "dir", ".", "project directory containing .kdef files")
	applyCmd.Flags().StringSliceVar(&applySetVars, "set", nil, "set variable values (key=value)")
	applyCmd.Flags().StringVar(&applyValuesFile, "values", "", "JSON values file for complex variables")
	applyCmd.Flags().StringVar(&applyEnv, "env", "", "environment name (loads environments/<env>.kdef)")
	applyCmd.Flags().StringSliceVar(&applyVarsFrom, "vars-from", nil, "import variable files (path to .kdef)")
	applyCmd.Flags().BoolVar(&applyDryRun, "dry-run", false, "preview changes without applying")
}

func runApply(cmd *cobra.Command, args []string) error {
	overrides := parseSetFlags(applySetVars)

	config, err := parser.LoadWithOptions(parser.LoadOptions{
		Dir:        applyDir,
		Overrides:  overrides,
		ValuesFile: applyValuesFile,
		Env:        applyEnv,
		VarsFrom:   applyVarsFrom,
	})
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	allManifests := generator.Generate(config)
	yamlBytes, err := generator.RenderAllYAML(allManifests)
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}

	// Write to temp file
	tmpFile, err := os.CreateTemp("", "kdef-apply-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(yamlBytes); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()

	// Build kubectl command
	kubectlArgs := []string{"apply", "--server-side", "--force-conflicts", "-f", tmpFile.Name()}
	if applyDryRun {
		kubectlArgs = append(kubectlArgs, "--dry-run=server")
	}

	kubectlCmd := exec.Command("kubectl", kubectlArgs...)
	kubectlCmd.Stdout = cmd.OutOrStdout()
	kubectlCmd.Stderr = cmd.ErrOrStderr()

	if err := kubectlCmd.Run(); err != nil {
		return fmt.Errorf("kubectl apply: %w", err)
	}

	return nil
}
