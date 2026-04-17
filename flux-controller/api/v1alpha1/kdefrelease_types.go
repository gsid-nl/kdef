package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KdefReleaseSpec defines the desired state of a KdefRelease.
type KdefReleaseSpec struct {
	// SourceRef points to a Flux source (GitRepository or OCIRepository).
	SourceRef CrossNamespaceSourceReference `json:"sourceRef"`

	// Path within the source artifact to the kdef project directory.
	// +optional
	Path string `json:"path,omitempty"`

	// Env selects the kdef environment (loads environments/<env>.kdef).
	// +optional
	Env string `json:"env,omitempty"`

	// Set provides --set key=value overrides for kdef variables.
	// +optional
	Set map[string]string `json:"set,omitempty"`

	// ValuesFrom references a ConfigMap or Secret containing a JSON values file.
	// +optional
	ValuesFrom *ValuesReference `json:"valuesFrom,omitempty"`

	// Interval at which to reconcile the KdefRelease.
	Interval metav1.Duration `json:"interval"`

	// Prune enables garbage collection of resources removed from output.
	// +optional
	Prune bool `json:"prune,omitempty"`

	// TargetNamespace overrides the namespace for all rendered resources.
	// +optional
	TargetNamespace string `json:"targetNamespace,omitempty"`

	// ServiceAccountName for impersonation during apply.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Suspend tells the controller to suspend reconciliation.
	// +optional
	Suspend bool `json:"suspend,omitempty"`
}

// CrossNamespaceSourceReference holds a reference to a Flux source object.
type CrossNamespaceSourceReference struct {
	// Kind of the source (GitRepository, OCIRepository, Bucket).
	Kind string `json:"kind"`

	// Name of the source object.
	Name string `json:"name"`

	// Namespace of the source object. Defaults to the KdefRelease namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ValuesReference holds a reference to a ConfigMap or Secret containing values.
type ValuesReference struct {
	// Kind of the values source: ConfigMap or Secret.
	Kind string `json:"kind"`

	// Name of the ConfigMap or Secret.
	Name string `json:"name"`

	// Key in the ConfigMap/Secret data. Defaults to "values.json".
	// +optional
	Key string `json:"key,omitempty"`
}

// KdefReleaseStatus defines the observed state of a KdefRelease.
type KdefReleaseStatus struct {
	// Conditions holds the conditions for the KdefRelease.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastAppliedRevision is the source revision that was last applied.
	// +optional
	LastAppliedRevision string `json:"lastAppliedRevision,omitempty"`

	// LastAttemptedRevision is the source revision that was last attempted.
	// +optional
	LastAttemptedRevision string `json:"lastAttemptedRevision,omitempty"`

	// Inventory contains the list of applied resources for pruning.
	// +optional
	Inventory *ResourceInventory `json:"inventory,omitempty"`

	// ObservedGeneration is the last observed generation of the KdefRelease.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastHandledReconcileAt holds the value of the reconcile request annotation
	// that was last handled.
	// +optional
	LastHandledReconcileAt string `json:"lastHandledReconcileAt,omitempty"`
}

// ResourceInventory contains a list of Kubernetes resource references.
type ResourceInventory struct {
	// Entries of applied resources.
	Entries []ResourceRef `json:"entries"`
}

// ResourceRef holds the identity of a Kubernetes resource.
type ResourceRef struct {
	// ID is namespace_name_group_kind.
	ID string `json:"id"`

	// Version is the API version.
	Version string `json:"v"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].message"
// +kubebuilder:printcolumn:name="Revision",type="string",JSONPath=".status.lastAppliedRevision"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// KdefRelease is the Schema for the kdefreleases API.
type KdefRelease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KdefReleaseSpec   `json:"spec,omitempty"`
	Status KdefReleaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KdefReleaseList contains a list of KdefRelease.
type KdefReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KdefRelease `json:"items"`
}
