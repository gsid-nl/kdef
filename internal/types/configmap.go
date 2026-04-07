package types

// ConfigMapConfig represents a parsed configmap block.
type ConfigMapConfig struct {
	Name      string
	Namespace string
	Data      map[string]string
}
