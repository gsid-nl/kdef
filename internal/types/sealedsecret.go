package types

// SealedSecretConfig represents a parsed sealedsecret block.
type SealedSecretConfig struct {
	Name      string
	Namespace string
	Type      string // e.g. "Opaque", "kubernetes.io/tls" (default: "Opaque")
	Data      map[string]string
}
