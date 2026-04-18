package importer

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// renderDeploymentBlock renders a deployment group using the new deployment + container syntax.
func renderDeploymentBlock(group AppGroup) string {
	dep := group.Deployment
	podSpec := dep.Spec.Template.Spec
	var b strings.Builder

	b.WriteString(fmt.Sprintf("deployment %q {\n", group.Name))

	if dep.Namespace != "" {
		b.WriteString(fmt.Sprintf("  namespace = %q\n", dep.Namespace))
	}

	// Selector
	if dep.Spec.Selector != nil {
		sel := dep.Spec.Selector.MatchLabels
		_, hasStdLabel := sel["app.kubernetes.io/name"]
		if !hasStdLabel || len(sel) != 1 {
			b.WriteString("  selector = {\n")
			for _, k := range sortedKeys(sel) {
				b.WriteString(fmt.Sprintf("    %q = %q\n", k, sel[k]))
			}
			b.WriteString("  }\n")

			podLabels := dep.Spec.Template.Labels
			if len(podLabels) > 0 {
				b.WriteString("  labels = {\n")
				for _, k := range sortedKeys(podLabels) {
					b.WriteString(fmt.Sprintf("    %q = %q\n", k, podLabels[k]))
				}
				b.WriteString("  }\n")
			}
		}
	}

	// ImagePullSecrets
	if len(podSpec.ImagePullSecrets) > 0 {
		var secrets []string
		for _, s := range podSpec.ImagePullSecrets {
			secrets = append(secrets, fmt.Sprintf("%q", s.Name))
		}
		b.WriteString(fmt.Sprintf("  image_pull_secrets = [%s]\n", strings.Join(secrets, ", ")))
	}

	// ServiceAccount
	if podSpec.ServiceAccountName != "" && podSpec.ServiceAccountName != "default" {
		b.WriteString(fmt.Sprintf("  service_account = %q\n", podSpec.ServiceAccountName))
	}

	// Scale
	if dep.Spec.Replicas != nil && *dep.Spec.Replicas != 1 {
		b.WriteString(fmt.Sprintf("\n  scale {\n    replicas = %d\n  }\n", *dep.Spec.Replicas))
	}

	// Containers
	for _, c := range podSpec.Containers {
		renderContainerBlock(&b, c, podSpec.Volumes)
	}

	// Init containers
	for _, ic := range podSpec.InitContainers {
		renderInitContainerBlock(&b, ic)
	}

	// Pod-level volumes are derived from container volume blocks automatically
	// (the generator deduplicates them)

	// Pod-level security context
	if podSpec.SecurityContext != nil && podSpec.SecurityContext.FSGroup != nil {
		b.WriteString("\n  security_context {\n")
		b.WriteString(fmt.Sprintf("    fs_group = %d\n", *podSpec.SecurityContext.FSGroup))
		b.WriteString("  }\n")
	}

	writeNodeSelector(&b, podSpec.NodeSelector)
	writeHostPodFlags(&b, podSpec)
	writeTolerations(&b, podSpec.Tolerations)

	// Service
	if group.Service != nil {
		renderServiceBlock(&b, group.Service, group.Name)
	}

	// Ingress
	if group.Ingress != nil {
		writeIngress(&b, group.Ingress, group)
	}

	b.WriteString("}\n")
	return b.String()
}

func renderContainerBlock(b *strings.Builder, c corev1.Container, podVolumes []corev1.Volume) {
	b.WriteString(fmt.Sprintf("\n  container %q {\n", c.Name))
	b.WriteString(fmt.Sprintf("    image = %q\n", c.Image))

	if c.ImagePullPolicy != "" && c.ImagePullPolicy != "IfNotPresent" {
		b.WriteString(fmt.Sprintf("    image_pull_policy = %q\n", string(c.ImagePullPolicy)))
	}

	writeCommandMultiLine(b, c.Command, "    ")
	writeArgsMultiLine(b, c.Args, "    ")

	// Ports
	for _, port := range c.Ports {
		portName := port.Name
		if portName == "" {
			portName = fmt.Sprintf("port-%d", port.ContainerPort)
		}
		b.WriteString(fmt.Sprintf("\n    port %q %q {\n", fmt.Sprintf("%d", port.ContainerPort), portName))
		if c.LivenessProbe != nil {
			if c.LivenessProbe.HTTPGet != nil {
				b.WriteString(fmt.Sprintf("      health = %q\n", c.LivenessProbe.HTTPGet.Path))
			} else if c.LivenessProbe.TCPSocket != nil {
				b.WriteString("      tcp_health = true\n")
			}
		}
		if c.ReadinessProbe != nil {
			if c.ReadinessProbe.HTTPGet != nil {
				b.WriteString(fmt.Sprintf("      ready  = %q\n", c.ReadinessProbe.HTTPGet.Path))
			} else if c.ReadinessProbe.TCPSocket != nil {
				b.WriteString("      tcp_ready = true\n")
			}
		}
		writeProbeSettings(b, c.LivenessProbe, "      ")
		b.WriteString("    }\n")
	}

	// Env
	writeEnv(b, c, "    ")

	// EnvFrom
	writeEnvFrom(b, c, "    ")

	// Resources
	writeResources(b, c, "    ")

	// Volume mounts (container-specific)
	writeVolumes(b, podVolumes, c.VolumeMounts, "    ")

	// Security context
	writeSecurityContext(b, c.SecurityContext, nil, "    ")

	b.WriteString("  }\n")
}

// renderInitContainerBlock renders an init container shared across workload kinds.
func renderInitContainerBlock(b *strings.Builder, ic corev1.Container) {
	b.WriteString(fmt.Sprintf("\n  init %q {\n", ic.Name))
	b.WriteString(fmt.Sprintf("    image = %q\n", ic.Image))
	if ic.ImagePullPolicy != "" && ic.ImagePullPolicy != "IfNotPresent" {
		b.WriteString(fmt.Sprintf("    image_pull_policy = %q\n", string(ic.ImagePullPolicy)))
	}
	writeCommandMultiLine(b, ic.Command, "    ")
	writeArgsMultiLine(b, ic.Args, "    ")
	if len(ic.VolumeMounts) > 0 {
		var volNames []string
		for _, vm := range ic.VolumeMounts {
			volNames = append(volNames, fmt.Sprintf("%q", vm.Name))
		}
		b.WriteString(fmt.Sprintf("    volumes = [%s]\n", strings.Join(volNames, ", ")))
	}
	writeEnvFrom(b, ic, "    ")
	if ic.SecurityContext != nil {
		writeSecurityContext(b, ic.SecurityContext, nil, "    ")
	}
	b.WriteString("  }\n")
}

// renderServiceBlock renders a Service block shared across workload kinds.
// The workloadName is used to suppress the service name when it's implicit.
func renderServiceBlock(b *strings.Builder, svc *corev1.Service, workloadName string) {
	b.WriteString("\n  service {\n")
	if svc.Name != workloadName {
		b.WriteString(fmt.Sprintf("    name = %q\n", svc.Name))
	}
	for _, port := range svc.Spec.Ports {
		portName := port.Name
		if portName == "" {
			portName = fmt.Sprintf("port-%d", port.Port)
		}
		if port.TargetPort.IntValue() != 0 && port.TargetPort.IntValue() != int(port.Port) {
			b.WriteString(fmt.Sprintf("    port %q %q {\n      target = %d\n    }\n",
				fmt.Sprintf("%d", port.Port), portName, port.TargetPort.IntValue()))
		} else {
			b.WriteString(fmt.Sprintf("    port %q %q {}\n", fmt.Sprintf("%d", port.Port), portName))
		}
	}
	b.WriteString("  }\n")
}

// writePodVolumes writes shared pod-level volumes (without mount paths, just the source definition).
func writePodVolumes(b *strings.Builder, volumes []corev1.Volume) {
	for _, vol := range volumes {
		b.WriteString(fmt.Sprintf("\n  volume %q {\n", vol.Name))
		// We need a mount_path but pod-level volumes don't have one.
		// For now, just write the source. The mount_path comes from the container's volume block.
		switch {
		case vol.Secret != nil:
			b.WriteString(fmt.Sprintf("    mount_path = \"/mnt/%s\"\n", vol.Name))
			b.WriteString(fmt.Sprintf("    secret = %q\n", vol.Secret.SecretName))
		case vol.ConfigMap != nil:
			b.WriteString(fmt.Sprintf("    mount_path = \"/mnt/%s\"\n", vol.Name))
			b.WriteString(fmt.Sprintf("    config_map = %q\n", vol.ConfigMap.Name))
		case vol.EmptyDir != nil:
			b.WriteString(fmt.Sprintf("    mount_path = \"/mnt/%s\"\n", vol.Name))
			b.WriteString("    empty_dir = true\n")
		case vol.PersistentVolumeClaim != nil:
			b.WriteString(fmt.Sprintf("    mount_path = \"/mnt/%s\"\n", vol.Name))
			b.WriteString(fmt.Sprintf("    pvc = %q\n", vol.PersistentVolumeClaim.ClaimName))
		case vol.HostPath != nil:
			b.WriteString(fmt.Sprintf("    mount_path = \"/mnt/%s\"\n", vol.Name))
			b.WriteString(fmt.Sprintf("    host_path = %q\n", vol.HostPath.Path))
		}
		b.WriteString("  }\n")
	}
}
