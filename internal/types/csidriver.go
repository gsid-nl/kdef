package types

// CSIDriverConfig represents a parsed csidriver block (storage.k8s.io/v1
// CSIDriver): how kubelet talks to a CSI driver.
type CSIDriverConfig struct {
	Name              string
	AttachRequired    bool
	PodInfoOnMount    bool
	RequiresRepublish bool
	FSGroupPolicy     string
	LifecycleModes    []string // "Persistent", "Ephemeral"
}
