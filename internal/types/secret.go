package types

// SecretConfig represents a parsed secret block.
type SecretConfig struct {
	Name      string
	Namespace string
	Type      string // e.g. "Opaque", "kubernetes.io/tls" (default: "Opaque")
	Data      map[string]string
}
