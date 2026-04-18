package importer

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// renderStatefulSetBlock renders a statefulset group as a kdef statefulset block.
func renderStatefulSetBlock(group AppGroup) string {
	sts := group.StatefulSet
	podSpec := sts.Spec.Template.Spec
	var b strings.Builder

	b.WriteString(fmt.Sprintf("statefulset %q {\n", group.Name))

	if sts.Namespace != "" {
		b.WriteString(fmt.Sprintf("  namespace = %q\n", sts.Namespace))
	}

	if sts.Spec.ServiceName != "" {
		b.WriteString(fmt.Sprintf("  service_name = %q\n", sts.Spec.ServiceName))
	}

	if sts.Spec.PodManagementPolicy != "" && sts.Spec.PodManagementPolicy != "OrderedReady" {
		b.WriteString(fmt.Sprintf("  pod_management_policy = %q\n", string(sts.Spec.PodManagementPolicy)))
	}

	// Selector
	if sts.Spec.Selector != nil {
		sel := sts.Spec.Selector.MatchLabels
		_, hasStdLabel := sel["app.kubernetes.io/name"]
		if !hasStdLabel || len(sel) != 1 {
			b.WriteString("  selector = {\n")
			for _, k := range sortedKeys(sel) {
				b.WriteString(fmt.Sprintf("    %q = %q\n", k, sel[k]))
			}
			b.WriteString("  }\n")

			podLabels := sts.Spec.Template.Labels
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

	if sts.Spec.Replicas != nil && *sts.Spec.Replicas != 1 {
		b.WriteString(fmt.Sprintf("\n  scale {\n    replicas = %d\n  }\n", *sts.Spec.Replicas))
	}

	for _, c := range podSpec.Containers {
		renderContainerBlock(&b, c, podSpec.Volumes)
	}

	for _, ic := range podSpec.InitContainers {
		renderInitContainerBlock(&b, ic)
	}

	// Volume claim templates — match each template against containers that mount it
	for _, vct := range sts.Spec.VolumeClaimTemplates {
		mountPath, subPath, readOnly := findMountForVolume(podSpec.Containers, vct.Name)
		b.WriteString(fmt.Sprintf("\n  volume_claim %q {\n", vct.Name))
		if mountPath != "" {
			b.WriteString(fmt.Sprintf("    mount_path = %q\n", mountPath))
		}
		if subPath != "" {
			b.WriteString(fmt.Sprintf("    sub_path   = %q\n", subPath))
		}
		if readOnly {
			b.WriteString("    read_only  = true\n")
		}
		if vct.Spec.StorageClassName != nil && *vct.Spec.StorageClassName != "" {
			b.WriteString(fmt.Sprintf("    storage_class = %q\n", *vct.Spec.StorageClassName))
		}
		if len(vct.Spec.AccessModes) > 0 {
			var modes []string
			for _, m := range vct.Spec.AccessModes {
				modes = append(modes, fmt.Sprintf("%q", string(m)))
			}
			b.WriteString(fmt.Sprintf("    access_modes  = [%s]\n", strings.Join(modes, ", ")))
		}
		if storage, ok := vct.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			b.WriteString(fmt.Sprintf("    storage       = %q\n", storage.String()))
		}
		b.WriteString("  }\n")
	}

	if podSpec.SecurityContext != nil && podSpec.SecurityContext.FSGroup != nil {
		b.WriteString("\n  security_context {\n")
		b.WriteString(fmt.Sprintf("    fs_group = %d\n", *podSpec.SecurityContext.FSGroup))
		b.WriteString("  }\n")
	}

	if group.Service != nil {
		renderServiceBlock(&b, group.Service, group.Name)
	}

	if group.Ingress != nil {
		writeIngress(&b, group.Ingress, group)
	}

	b.WriteString("}\n")
	return b.String()
}

// findMountForVolume returns the mount path, sub path, and read_only flag from
// the first container that mounts the named volume.
func findMountForVolume(containers []corev1.Container, name string) (string, string, bool) {
	for _, c := range containers {
		for _, vm := range c.VolumeMounts {
			if vm.Name == name {
				return vm.MountPath, vm.SubPath, vm.ReadOnly
			}
		}
	}
	return "", "", false
}
