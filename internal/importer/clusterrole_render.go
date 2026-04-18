package importer

import (
	"fmt"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
)

func renderClusterRoleBlock(cr rbacv1.ClusterRole) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("clusterrole %q {\n", cr.Name))
	for _, rule := range cr.Rules {
		b.WriteString("\n  rule {\n")
		writeStringList(&b, "    api_groups", rule.APIGroups)
		writeStringList(&b, "    resources", rule.Resources)
		writeStringList(&b, "    resource_names", rule.ResourceNames)
		writeStringList(&b, "    verbs", rule.Verbs)
		writeStringList(&b, "    non_resource_urls", rule.NonResourceURLs)
		b.WriteString("  }\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func renderClusterRoleBindingBlock(crb rbacv1.ClusterRoleBinding) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("clusterrolebinding %q {\n", crb.Name))

	b.WriteString("\n  role_ref {\n")
	if crb.RoleRef.Kind != "" {
		b.WriteString(fmt.Sprintf("    kind = %q\n", crb.RoleRef.Kind))
	}
	b.WriteString(fmt.Sprintf("    name = %q\n", crb.RoleRef.Name))
	b.WriteString("  }\n")

	for _, s := range crb.Subjects {
		b.WriteString("\n  subject {\n")
		if s.Kind != "" {
			b.WriteString(fmt.Sprintf("    kind      = %q\n", s.Kind))
		}
		b.WriteString(fmt.Sprintf("    name      = %q\n", s.Name))
		if s.Namespace != "" {
			b.WriteString(fmt.Sprintf("    namespace = %q\n", s.Namespace))
		}
		b.WriteString("  }\n")
	}

	b.WriteString("}\n")
	return b.String()
}

func writeStringList(b *strings.Builder, prefix string, vals []string) {
	if len(vals) == 0 {
		return
	}
	var quoted []string
	for _, v := range vals {
		quoted = append(quoted, fmt.Sprintf("%q", v))
	}
	b.WriteString(fmt.Sprintf("%s = [%s]\n", prefix, strings.Join(quoted, ", ")))
}
