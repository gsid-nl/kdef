package types

// ClusterRoleConfig represents a parsed clusterrole block.
type ClusterRoleConfig struct {
	Name  string
	Rules []PolicyRuleConfig
}

// PolicyRuleConfig represents a single RBAC policy rule.
type PolicyRuleConfig struct {
	APIGroups       []string
	Resources       []string
	ResourceNames   []string
	Verbs           []string
	NonResourceURLs []string
}

// ClusterRoleBindingConfig represents a parsed clusterrolebinding block.
type ClusterRoleBindingConfig struct {
	Name     string
	RoleRef  RoleRefConfig
	Subjects []SubjectConfig
}

// RoleRefConfig identifies the ClusterRole or Role the binding references.
type RoleRefConfig struct {
	Kind string // "ClusterRole" or "Role"
	Name string
}

// SubjectConfig identifies a ServiceAccount, User, or Group subject.
type SubjectConfig struct {
	Kind      string // "ServiceAccount", "User", or "Group"
	Name      string
	Namespace string // required for ServiceAccount in ClusterRoleBinding
}
