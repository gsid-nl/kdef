package types

// PersistentVolumeClaimConfig represents a parsed persistentvolumeclaim block.
type PersistentVolumeClaimConfig struct {
	Name         string
	Namespace    string
	StorageClass string
	AccessModes  []string // e.g. "ReadWriteOnce", "ReadOnlyMany", "ReadWriteMany"
	Storage      string   // e.g. "10Gi"
}
