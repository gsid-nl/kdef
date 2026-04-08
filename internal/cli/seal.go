package cli

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var sealCmd = &cobra.Command{
	Use:   "seal",
	Short: "Encrypt values using kubeseal for use in sealedsecret blocks",
	Long: `Encrypt secret values using kubeseal (Bitnami Sealed Secrets).

The encrypted output can be pasted directly into a sealedsecret block's data map.
Requires kubeseal to be installed and a sealed-secrets controller running in the cluster.

Examples:
  # Encrypt a single value (reads from --value or stdin)
  kdef seal --secret db-credentials --key PASSWORD --value "hunter2"

  # Encrypt from stdin
  echo -n "hunter2" | kdef seal --secret db-credentials --key PASSWORD

  # Specify namespace and controller
  kdef seal --secret db-credentials --key PASSWORD --value "hunter2" \
    --namespace production --controller-name sealed-secrets`,
	RunE: runSeal,
}

var (
	sealSecretName     string
	sealKey            string
	sealValue          string
	sealNamespace      string
	sealControllerName string
	sealControllerNs   string
)

func init() {
	sealCmd.Flags().StringVar(&sealSecretName, "secret", "", "name of the Secret to encrypt for (required)")
	sealCmd.Flags().StringVar(&sealKey, "key", "", "key within the Secret to encrypt (required)")
	sealCmd.Flags().StringVar(&sealValue, "value", "", "plaintext value to encrypt (reads stdin if omitted)")
	sealCmd.Flags().StringVar(&sealNamespace, "namespace", "default", "namespace of the Secret")
	sealCmd.Flags().StringVar(&sealControllerName, "controller-name", "", "name of the sealed-secrets controller")
	sealCmd.Flags().StringVar(&sealControllerNs, "controller-namespace", "", "namespace of the sealed-secrets controller")

	_ = sealCmd.MarkFlagRequired("secret")
	_ = sealCmd.MarkFlagRequired("key")
}

func runSeal(cmd *cobra.Command, args []string) error {
	if _, err := exec.LookPath("kubeseal"); err != nil {
		return fmt.Errorf("kubeseal not found in PATH: install it from https://github.com/bitnami-labs/sealed-secrets/releases")
	}

	// Get the plaintext value
	var plaintext []byte
	if sealValue != "" {
		plaintext = []byte(sealValue)
	} else {
		// Read from stdin
		var err error
		plaintext, err = readStdin()
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		if len(plaintext) == 0 {
			return fmt.Errorf("no value provided: use --value or pipe to stdin")
		}
	}

	// Build kubeseal command for raw mode (encrypts a single value)
	kubesealArgs := []string{
		"--raw",
		"--name", sealSecretName,
		"--namespace", sealNamespace,
		"--from-file=/dev/stdin",
	}
	if sealControllerName != "" {
		kubesealArgs = append(kubesealArgs, "--controller-name", sealControllerName)
	}
	if sealControllerNs != "" {
		kubesealArgs = append(kubesealArgs, "--controller-namespace", sealControllerNs)
	}

	kubeseal := exec.Command("kubeseal", kubesealArgs...)
	kubeseal.Stdin = strings.NewReader(string(plaintext))
	kubeseal.Stderr = cmd.ErrOrStderr()

	output, err := kubeseal.Output()
	if err != nil {
		return fmt.Errorf("kubeseal failed: %w", err)
	}

	encrypted := strings.TrimSpace(string(output))

	// Validate it looks like base64
	if _, err := base64.StdEncoding.DecodeString(encrypted); err != nil {
		// kubeseal may output non-standard base64, just use as-is
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: output may not be standard base64\n")
	}

	// Print the result in a copy-pasteable format
	fmt.Fprintf(cmd.OutOrStdout(), "%s\n", encrypted)
	fmt.Fprintf(cmd.ErrOrStderr(), "\nUsage in .kdef:\n")
	fmt.Fprintf(cmd.ErrOrStderr(), "  sealedsecret %q {\n", sealSecretName)
	fmt.Fprintf(cmd.ErrOrStderr(), "    namespace = %q\n", sealNamespace)
	fmt.Fprintf(cmd.ErrOrStderr(), "    data = {\n")
	fmt.Fprintf(cmd.ErrOrStderr(), "      %s = %q\n", sealKey, encrypted)
	fmt.Fprintf(cmd.ErrOrStderr(), "    }\n")
	fmt.Fprintf(cmd.ErrOrStderr(), "  }\n")

	return nil
}

func readStdin() ([]byte, error) {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		// stdin is a terminal, not a pipe
		return nil, nil
	}
	return os.ReadFile("/dev/stdin")
}
