package importer

import (
	"fmt"
	"strings"
)

// renderDaemonSetBlock renders a daemonset group as a kdef daemonset block.
func renderDaemonSetBlock(group AppGroup) string {
	ds := group.DaemonSet
	podSpec := ds.Spec.Template.Spec
	var b strings.Builder

	b.WriteString(fmt.Sprintf("daemonset %q {\n", group.Name))

	if ds.Namespace != "" {
		b.WriteString(fmt.Sprintf("  namespace = %q\n", ds.Namespace))
	}

	// Selector
	if ds.Spec.Selector != nil {
		sel := ds.Spec.Selector.MatchLabels
		_, hasStdLabel := sel["app.kubernetes.io/name"]
		if !hasStdLabel || len(sel) != 1 {
			b.WriteString("  selector = {\n")
			for _, k := range sortedKeys(sel) {
				b.WriteString(fmt.Sprintf("    %q = %q\n", k, sel[k]))
			}
			b.WriteString("  }\n")

			podLabels := ds.Spec.Template.Labels
			if len(podLabels) > 0 {
				b.WriteString("  labels = {\n")
				for _, k := range sortedKeys(podLabels) {
					b.WriteString(fmt.Sprintf("    %q = %q\n", k, podLabels[k]))
				}
				b.WriteString("  }\n")
			}
		}
	}

	if len(podSpec.ImagePullSecrets) > 0 {
		var secrets []string
		for _, s := range podSpec.ImagePullSecrets {
			secrets = append(secrets, fmt.Sprintf("%q", s.Name))
		}
		b.WriteString(fmt.Sprintf("  image_pull_secrets = [%s]\n", strings.Join(secrets, ", ")))
	}

	if podSpec.ServiceAccountName != "" && podSpec.ServiceAccountName != "default" {
		b.WriteString(fmt.Sprintf("  service_account = %q\n", podSpec.ServiceAccountName))
	}

	for _, c := range podSpec.Containers {
		renderContainerBlock(&b, c, podSpec.Volumes)
	}

	for _, ic := range podSpec.InitContainers {
		renderInitContainerBlock(&b, ic)
	}

	if podSpec.SecurityContext != nil && podSpec.SecurityContext.FSGroup != nil {
		b.WriteString("\n  security_context {\n")
		b.WriteString(fmt.Sprintf("    fs_group = %d\n", *podSpec.SecurityContext.FSGroup))
		b.WriteString("  }\n")
	}

	writeNodeSelector(&b, podSpec.NodeSelector)
	writeHostPodFlags(&b, podSpec)
	writeTolerations(&b, podSpec.Tolerations)

	if group.Service != nil {
		renderServiceBlock(&b, group.Service, group.Name)
	}

	b.WriteString("}\n")
	return b.String()
}
