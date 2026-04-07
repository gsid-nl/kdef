package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/gsid-nl/kdef/internal/parser"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate .kdef files for type errors and missing references",
	RunE:  runValidate,
}

var (
	validateDir        string
	validateSetVars    []string
	validateValuesFile string
	validateEnv        string
)

func init() {
	validateCmd.Flags().StringVar(&validateDir, "dir", ".", "project directory containing .kdef files")
	validateCmd.Flags().StringSliceVar(&validateSetVars, "set", nil, "set variable values (key=value)")
	validateCmd.Flags().StringVar(&validateValuesFile, "values", "", "JSON values file for complex variables")
	validateCmd.Flags().StringVar(&validateEnv, "env", "", "environment name (loads environments/<env>.kdef)")
}

func runValidate(cmd *cobra.Command, args []string) error {
	overrides := parseSetFlags(validateSetVars)

	config, err := parser.LoadWithOptions(parser.LoadOptions{
		Dir:        validateDir,
		Overrides:  overrides,
		ValuesFile: validateValuesFile,
		Env:        validateEnv,
	})
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	var errors []string

	// Validate enum constraints
	for name, v := range config.Variables {
		if len(v.AllowedValues) > 0 && v.Default != nil {
			defaultStr := v.Default.AsString()
			if !contains(v.AllowedValues, defaultStr) {
				errors = append(errors, fmt.Sprintf(
					"variable %q: default value %q not in allowed values %v",
					name, defaultStr, v.AllowedValues,
				))
			}
		}
	}

	// Validate apps
	if len(errors) > 0 {
		for _, e := range errors {
			fmt.Fprintf(cmd.ErrOrStderr(), "error: %s\n", e)
		}
		return fmt.Errorf("validation failed with %d error(s)", len(errors))
	}

	total := len(config.Deployments) + len(config.CronJobs) + len(config.ConfigMaps)
	fmt.Fprintf(cmd.OutOrStdout(), "valid: %d deployment(s), %d cronjob(s), %d configmap(s) in %s\n",
		len(config.Deployments), len(config.CronJobs), len(config.ConfigMaps), validateDir)
	_ = total
	return nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
