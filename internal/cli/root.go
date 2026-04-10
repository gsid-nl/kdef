package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/gsid-nl/kdef/internal/version"
)

var rootCmd = &cobra.Command{
	Use:   "kdef",
	Short: "Declarative Kubernetes configuration language",
	Long: `kdef compiles declarative .kdef files into standard Kubernetes YAML manifests.

Block types:
  deployment     K8s Deployment with explicit containers, service, and ingress
  cronjob        K8s CronJob
  configmap      K8s ConfigMap
  sealedsecret   Bitnami SealedSecret (encrypted secrets safe to commit)`,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the kdef version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("kdef %s (commit: %s, built: %s)\n", version.Version, version.Commit, version.Date)
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.Version = version.Version
	rootCmd.AddCommand(renderCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(applyCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(sealCmd)
	rootCmd.AddCommand(sealSecretCmd)
	rootCmd.AddCommand(installHookCmd)
	rootCmd.AddCommand(versionCmd)
}
