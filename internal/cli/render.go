package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gsid-nl/kdef/internal/generator"
	"github.com/gsid-nl/kdef/internal/parser"
)

var renderCmd = &cobra.Command{
	Use:   "render",
	Short: "Render .kdef files to Kubernetes YAML",
	RunE:  runRender,
}

var (
	renderDir        string
	renderOutputDir  string
	renderSetVars    []string
	renderValuesFile string
	renderEnv        string
	renderVarsFrom   []string
)

func init() {
	renderCmd.Flags().StringVar(&renderDir, "dir", ".", "project directory containing .kdef files")
	renderCmd.Flags().StringVar(&renderOutputDir, "output-dir", "", "write YAML files to this directory (default: stdout)")
	renderCmd.Flags().StringSliceVar(&renderSetVars, "set", nil, "set variable values (key=value)")
	renderCmd.Flags().StringVar(&renderValuesFile, "values", "", "JSON values file for complex variables")
	renderCmd.Flags().StringVar(&renderEnv, "env", "", "environment name (loads environments/<env>.kdef)")
	renderCmd.Flags().StringSliceVar(&renderVarsFrom, "vars-from", nil, "import variable files (path to .kdef)")
}

func parseSetFlags(flags []string) map[string]string {
	overrides := make(map[string]string)
	for _, f := range flags {
		parts := strings.SplitN(f, "=", 2)
		if len(parts) == 2 {
			overrides[parts[0]] = parts[1]
		}
	}
	return overrides
}

func runRender(cmd *cobra.Command, args []string) error {
	overrides := parseSetFlags(renderSetVars)

	config, err := parser.LoadWithOptions(parser.LoadOptions{
		Dir:        renderDir,
		Overrides:  overrides,
		ValuesFile: renderValuesFile,
		Env:        renderEnv,
		VarsFrom:   renderVarsFrom,
	})
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	if len(config.Deployments) == 0 && len(config.DaemonSets) == 0 && len(config.StatefulSets) == 0 && len(config.CronJobs) == 0 && len(config.ConfigMaps) == 0 && len(config.Secrets) == 0 && len(config.SealedSecrets) == 0 && len(config.PersistentVolumeClaims) == 0 && len(config.ClusterRoles) == 0 && len(config.ClusterRoleBindings) == 0 {
		return fmt.Errorf("no blocks found in %s", renderDir)
	}

	// Print cross-resource reference warnings
	for _, w := range config.Warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
	}

	allManifests := generator.Generate(config)

	if renderOutputDir != "" {
		if err := os.MkdirAll(renderOutputDir, 0755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}

		for name, manifests := range allManifests {
			data, err := generator.RenderYAML(manifests)
			if err != nil {
				return fmt.Errorf("render %s: %w", name, err)
			}

			outPath := filepath.Join(renderOutputDir, name+".yaml")
			if err := os.WriteFile(outPath, data, 0644); err != nil {
				return fmt.Errorf("write %s: %w", outPath, err)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "wrote %s\n", outPath)
		}
	} else {
		data, err := generator.RenderAllYAML(allManifests)
		if err != nil {
			return fmt.Errorf("render: %w", err)
		}
		fmt.Fprint(cmd.OutOrStdout(), string(data))
	}

	return nil
}
