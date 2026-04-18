package generator

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gsid-nl/kdef/internal/types"
)

// GenerateClusterRole creates a ClusterRole from a ClusterRoleConfig.
func GenerateClusterRole(cr types.ClusterRoleConfig) *rbacv1.ClusterRole {
	var rules []rbacv1.PolicyRule
	for _, r := range cr.Rules {
		rule := rbacv1.PolicyRule{
			APIGroups:       r.APIGroups,
			Resources:       r.Resources,
			ResourceNames:   r.ResourceNames,
			Verbs:           r.Verbs,
			NonResourceURLs: r.NonResourceURLs,
		}
		// Default APIGroups to [""] (core group) if not specified and resources is set
		if len(rule.APIGroups) == 0 && len(rule.Resources) > 0 {
			rule.APIGroups = []string{""}
		}
		rules = append(rules, rule)
	}

	return &rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "ClusterRole",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   cr.Name,
			Labels: kdefLabels(map[string]string{"app.kubernetes.io/name": cr.Name}),
		},
		Rules: rules,
	}
}

// GenerateClusterRoleBinding creates a ClusterRoleBinding from a ClusterRoleBindingConfig.
func GenerateClusterRoleBinding(crb types.ClusterRoleBindingConfig) *rbacv1.ClusterRoleBinding {
	var subjects []rbacv1.Subject
	for _, s := range crb.Subjects {
		subjects = append(subjects, rbacv1.Subject{
			Kind:      s.Kind,
			Name:      s.Name,
			Namespace: s.Namespace,
		})
	}

	return &rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "ClusterRoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   crb.Name,
			Labels: kdefLabels(map[string]string{"app.kubernetes.io/name": crb.Name}),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     crb.RoleRef.Kind,
			Name:     crb.RoleRef.Name,
		},
		Subjects: subjects,
	}
}
