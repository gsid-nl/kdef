package generator

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/gsid-nl/kdef/internal/types"
)

// PodTemplateInput is the common input for building a pod template shared by
// Deployment, DaemonSet, and StatefulSet workloads.
type PodTemplateInput struct {
	Containers         []types.ContainerConfig
	InitContainers     []types.InitContainerConfig
	Volumes            []types.VolumeConfig
	ImagePullSecrets   []string
	ServiceAccountName string
	SecurityContext    *types.SecurityContextConfig
	NodeSelector       map[string]string
	Tolerations        []types.TolerationConfig
}

// buildPodSpec assembles a PodSpec shared across workload kinds.
// extraClaimNames lists volume names that are provided by StatefulSet
// volumeClaimTemplates — those are NOT added to the pod's Volumes list
// but are still mounted by containers.
func buildPodSpec(in PodTemplateInput, extraClaimNames map[string]bool) corev1.PodSpec {
	var containers []corev1.Container
	for _, c := range in.Containers {
		containers = append(containers, buildContainerV2(c))
	}

	var initContainers []corev1.Container
	for _, ic := range in.InitContainers {
		initC := corev1.Container{
			Name:  ic.Name,
			Image: ic.Image,
		}
		if ic.ImagePullPolicy != "" {
			initC.ImagePullPolicy = corev1.PullPolicy(ic.ImagePullPolicy)
		}
		if len(ic.Command) > 0 {
			initC.Command = ic.Command
		}
		if len(ic.Args) > 0 {
			initC.Args = ic.Args
		}
		for _, e := range ic.Env {
			initC.Env = append(initC.Env, buildEnvVar(e))
		}
		for _, ef := range ic.EnvFrom {
			initC.EnvFrom = append(initC.EnvFrom, buildEnvFromSource(ef))
		}
		for _, volName := range ic.VolumeMounts {
			if v, ok := findVolumeByName(in, volName); ok {
				initC.VolumeMounts = append(initC.VolumeMounts, buildVolumeMount(v))
			}
		}
		if ic.SecurityContext != nil {
			initC.SecurityContext = buildSecurityContext(ic.SecurityContext)
		}
		initContainers = append(initContainers, initC)
	}

	volumes := buildVolumes(in.Volumes)
	for _, c := range in.Containers {
		for _, cv := range c.Volumes {
			if extraClaimNames[cv.Name] {
				continue // provided by volumeClaimTemplates
			}
			if !volumeExists(volumes, cv.Name) {
				volumes = append(volumes, buildVolume(cv))
			}
		}
	}

	var imagePullSecrets []corev1.LocalObjectReference
	for _, s := range in.ImagePullSecrets {
		imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{Name: s})
	}

	spec := corev1.PodSpec{
		Containers: containers,
	}
	if len(initContainers) > 0 {
		spec.InitContainers = initContainers
	}
	if len(volumes) > 0 {
		spec.Volumes = volumes
	}
	if len(imagePullSecrets) > 0 {
		spec.ImagePullSecrets = imagePullSecrets
	}
	if in.ServiceAccountName != "" {
		spec.ServiceAccountName = in.ServiceAccountName
	}
	if in.SecurityContext != nil && in.SecurityContext.FSGroup != nil {
		spec.SecurityContext = &corev1.PodSecurityContext{
			FSGroup: in.SecurityContext.FSGroup,
		}
	}
	if len(in.NodeSelector) > 0 {
		spec.NodeSelector = in.NodeSelector
	}
	if tols := buildTolerations(in.Tolerations); len(tols) > 0 {
		spec.Tolerations = tols
	}
	return spec
}

func findVolumeByName(in PodTemplateInput, name string) (types.VolumeConfig, bool) {
	for _, v := range in.Volumes {
		if v.Name == name {
			return v, true
		}
	}
	for _, c := range in.Containers {
		for _, v := range c.Volumes {
			if v.Name == name {
				return v, true
			}
		}
	}
	return types.VolumeConfig{}, false
}
