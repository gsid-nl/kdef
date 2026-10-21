package types

// CRDConfig represents a parsed crd block (a CustomResourceDefinition).
// Group and Plural come from the block label, which Kubernetes requires to be
// "<plural>.<group>".
type CRDConfig struct {
	Name       string
	Group      string
	Plural     string
	Kind       string
	Singular   string
	ListKind   string
	ShortNames []string
	Categories []string
	Scope      string // "Namespaced" or "Cluster"
	Versions   []CRDVersionConfig
}

// CRDVersionConfig represents one version of a CRD.
type CRDVersionConfig struct {
	Name              string
	Served            bool
	Storage           bool
	Schema            map[string]interface{} // openAPIV3Schema; nil accepts any fields
	StatusSubresource bool
	Columns           []CRDColumnConfig
}

// CRDColumnConfig represents an additional printer column for kubectl get.
type CRDColumnConfig struct {
	Name        string
	Type        string
	JSONPath    string
	Description string
	Format      string
	Priority    int64
}
