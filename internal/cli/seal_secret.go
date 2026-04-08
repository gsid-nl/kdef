package cli

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

var sealSecretCmd = &cobra.Command{
	Use:   "seal-secret",
	Short: "Seal an entire Kubernetes Secret into a sealedsecret block",
	Long: `Fetch a Kubernetes Secret (from the cluster or a YAML file), encrypt all
keys using kubeseal, and output a ready-to-use sealedsecret block.

Requires kubeseal to be installed and a sealed-secrets controller running in the cluster.

Examples:
  # From live cluster
  kdef seal-secret --name db-credentials --namespace production

  # From a YAML file
  kdef seal-secret --from-file secret.yaml

  # With custom controller
  kdef seal-secret --name db-credentials --namespace production \
    --controller-name sealed-secrets`,
	RunE: runSealSecret,
}

var (
	sealSecretSecretName     string
	sealSecretNamespace      string
	sealSecretFromFile       string
	sealSecretControllerName string
	sealSecretControllerNs   string
)

func init() {
	sealSecretCmd.Flags().StringVar(&sealSecretSecretName, "name", "", "name of the Secret to seal (from cluster)")
	sealSecretCmd.Flags().StringVar(&sealSecretNamespace, "namespace", "default", "namespace of the Secret")
	sealSecretCmd.Flags().StringVar(&sealSecretFromFile, "from-file", "", "read Secret from a YAML/JSON file instead of cluster")
	sealSecretCmd.Flags().StringVar(&sealSecretControllerName, "controller-name", "", "name of the sealed-secrets controller")
	sealSecretCmd.Flags().StringVar(&sealSecretControllerNs, "controller-namespace", "", "namespace of the sealed-secrets controller")
}

type k8sSecret struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Metadata   k8sMetadata       `json:"metadata"`
	Type       string            `json:"type"`
	Data       map[string]string `json:"data"`
}

type k8sMetadata struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func runSealSecret(cmd *cobra.Command, args []string) error {
	if _, err := exec.LookPath("kubeseal"); err != nil {
		return fmt.Errorf("kubeseal not found in PATH: install it from https://github.com/bitnami-labs/sealed-secrets/releases")
	}

	if sealSecretSecretName == "" && sealSecretFromFile == "" {
		return fmt.Errorf("provide --name (fetch from cluster) or --from-file (read from file)")
	}

	var secret k8sSecret
	var err error

	if sealSecretFromFile != "" {
		secret, err = readSecretFromFile(sealSecretFromFile)
	} else {
		secret, err = fetchSecretFromCluster(sealSecretSecretName, sealSecretNamespace)
	}
	if err != nil {
		return err
	}

	if len(secret.Data) == 0 {
		return fmt.Errorf("secret %q has no data", secret.Metadata.Name)
	}

	name := secret.Metadata.Name
	namespace := secret.Metadata.Namespace
	if namespace == "" {
		namespace = sealSecretNamespace
	}
	secretType := secret.Type
	if secretType == "" {
		secretType = "Opaque"
	}

	// Sort keys for deterministic output
	var keys []string
	for k := range secret.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Seal each key
	fmt.Fprintf(cmd.ErrOrStderr(), "Sealing %d keys from secret %q...\n", len(keys), name)

	encryptedData := make(map[string]string)
	for _, key := range keys {
		raw, err := base64.StdEncoding.DecodeString(secret.Data[key])
		if err != nil {
			return fmt.Errorf("decode base64 for key %q: %w", key, err)
		}

		encrypted, err := sealRawValue(string(raw), name, namespace, sealSecretControllerName, sealSecretControllerNs, cmd)
		if err != nil {
			return fmt.Errorf("seal key %q: %w", key, err)
		}

		encryptedData[key] = encrypted
		fmt.Fprintf(cmd.ErrOrStderr(), "  sealed %s\n", key)
	}

	// Output the sealedsecret block
	fmt.Fprintf(cmd.OutOrStdout(), "sealedsecret %q {\n", name)
	fmt.Fprintf(cmd.OutOrStdout(), "  namespace = %q\n", namespace)
	if secretType != "Opaque" {
		fmt.Fprintf(cmd.OutOrStdout(), "  type      = %q\n", secretType)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\n  data = {\n")
	for _, key := range keys {
		fmt.Fprintf(cmd.OutOrStdout(), "    %s = %q\n", key, encryptedData[key])
	}
	fmt.Fprintf(cmd.OutOrStdout(), "  }\n")
	fmt.Fprintf(cmd.OutOrStdout(), "}\n")

	return nil
}

func sealRawValue(plaintext, secretName, namespace, controllerName, controllerNs string, cmd *cobra.Command) (string, error) {
	kubesealArgs := []string{
		"--raw",
		"--name", secretName,
		"--namespace", namespace,
		"--from-file=/dev/stdin",
	}
	if controllerName != "" {
		kubesealArgs = append(kubesealArgs, "--controller-name", controllerName)
	}
	if controllerNs != "" {
		kubesealArgs = append(kubesealArgs, "--controller-namespace", controllerNs)
	}

	kubeseal := exec.Command("kubeseal", kubesealArgs...)
	kubeseal.Stdin = strings.NewReader(plaintext)
	kubeseal.Stderr = cmd.ErrOrStderr()

	output, err := kubeseal.Output()
	if err != nil {
		return "", fmt.Errorf("kubeseal: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

func fetchSecretFromCluster(name, namespace string) (k8sSecret, error) {
	kubectlCmd := exec.Command("kubectl", "get", "secret", name, "-n", namespace, "-o", "json")
	output, err := kubectlCmd.Output()
	if err != nil {
		return k8sSecret{}, fmt.Errorf("kubectl get secret %s -n %s: %w", name, namespace, err)
	}

	var secret k8sSecret
	if err := json.Unmarshal(output, &secret); err != nil {
		return k8sSecret{}, fmt.Errorf("parse secret JSON: %w", err)
	}

	return secret, nil
}

func readSecretFromFile(path string) (k8sSecret, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return k8sSecret{}, fmt.Errorf("read file %q: %w", path, err)
	}

	// sigs.k8s.io/yaml handles both YAML and JSON
	jsonData, err := yaml.YAMLToJSON(data)
	if err != nil {
		return k8sSecret{}, fmt.Errorf("parse %q: %w", path, err)
	}

	var secret k8sSecret
	if err := json.Unmarshal(jsonData, &secret); err != nil {
		return k8sSecret{}, fmt.Errorf("parse %q: %w", path, err)
	}

	if secret.Kind != "" && secret.Kind != "Secret" {
		return k8sSecret{}, fmt.Errorf("file %q contains a %s, not a Secret", path, secret.Kind)
	}

	return secret, nil
}
