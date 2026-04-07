package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/gsid-nl/kdef/internal/importer"
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import existing K8s manifests and generate .kdef files",
	Long: `Import reads Kubernetes resources from a live cluster or YAML files
and generates .kdef configuration files.

Examples:
  # Import from live cluster
  kdef import --namespace timepickr --output-dir k8s/

  # Import from YAML file
  kdef import --from-file manifests.yaml

  # Preview without writing files
  kdef import --namespace timepickr`,
	RunE: runImport,
}

var (
	importNamespace string
	importFromFile  string
	importOutputDir string
)

func init() {
	importCmd.Flags().StringVar(&importNamespace, "namespace", "", "Kubernetes namespace to import from")
	importCmd.Flags().StringVar(&importFromFile, "from-file", "", "YAML file to import from (instead of cluster)")
	importCmd.Flags().StringVar(&importOutputDir, "output-dir", "", "write .kdef files to this directory (default: print to stdout)")
}

func runImport(cmd *cobra.Command, args []string) error {
	var resources *importer.ClusterResources
	var err error

	if importFromFile != "" {
		resources, err = importer.ReadFromFile(importFromFile)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
	} else if importNamespace != "" {
		resources, err = importer.ReadFromCluster(importNamespace)
		if err != nil {
			return fmt.Errorf("read cluster: %w", err)
		}
	} else {
		return fmt.Errorf("specify --namespace or --from-file")
	}

	result := importer.MapToKdef(resources)

	total := len(result.Deployments) + len(result.CronJobs) + len(result.ConfigMaps)
	if total == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "no resources found to import")
		return nil
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "found %d deployment(s), %d cronjob(s), %d configmap(s)\n",
		len(result.Deployments), len(result.CronJobs), len(result.ConfigMaps))

	if importOutputDir != "" {
		return importer.WriteKdefFiles(result, importOutputDir)
	}

	importer.PrintKdef(result)
	return nil
}
