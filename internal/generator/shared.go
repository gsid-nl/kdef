package generator

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/gsid-nl/kdef/internal/types"
	"github.com/gsid-nl/kdef/internal/version"
)

// kdefLabels adds kdef managed-by and version labels to a label map.
func kdefLabels(labels map[string]string) map[string]string {
	labels["app.kubernetes.io/managed-by"] = "kdef"
	labels["kdef.gsid.nl/version"] = version.Version
	return labels
}

func buildEnvVar(e types.EnvEntry) corev1.EnvVar {
	envVar := corev1.EnvVar{Name: e.Name}
	if e.SecretName != "" {
		envVar.ValueFrom = &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: e.SecretName},
				Key:                  e.SecretKey,
			},
		}
	} else {
		envVar.Value = e.Value
	}
	return envVar
}

func buildEnvFromSource(ef types.EnvFromEntry) corev1.EnvFromSource {
	envFrom := corev1.EnvFromSource{Prefix: ef.Prefix}
	if ef.ConfigMap != "" {
		envFrom.ConfigMapRef = &corev1.ConfigMapEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: ef.ConfigMap},
		}
	}
	if ef.Secret != "" {
		envFrom.SecretRef = &corev1.SecretEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: ef.Secret},
		}
	}
	return envFrom
}

func buildResources(r *types.ResourcesConfig) corev1.ResourceRequirements {
	res := corev1.ResourceRequirements{}
	if r.CPURequest != "" || r.MemoryRequest != "" || r.EphemeralStorageRequest != "" {
		res.Requests = corev1.ResourceList{}
		if r.CPURequest != "" {
			res.Requests[corev1.ResourceCPU] = resource.MustParse(r.CPURequest)
		}
		if r.MemoryRequest != "" {
			res.Requests[corev1.ResourceMemory] = resource.MustParse(r.MemoryRequest)
		}
		if r.EphemeralStorageRequest != "" {
			res.Requests[corev1.ResourceEphemeralStorage] = resource.MustParse(r.EphemeralStorageRequest)
		}
	}
	if r.CPULimit != "" || r.MemoryLimit != "" || r.EphemeralStorageLimit != "" {
		res.Limits = corev1.ResourceList{}
		if r.CPULimit != "" {
			res.Limits[corev1.ResourceCPU] = resource.MustParse(r.CPULimit)
		}
		if r.MemoryLimit != "" {
			res.Limits[corev1.ResourceMemory] = resource.MustParse(r.MemoryLimit)
		}
		if r.EphemeralStorageLimit != "" {
			res.Limits[corev1.ResourceEphemeralStorage] = resource.MustParse(r.EphemeralStorageLimit)
		}
	}
	return res
}

func buildVolumeMount(v types.VolumeConfig) corev1.VolumeMount {
	vm := corev1.VolumeMount{
		Name:      v.Name,
		MountPath: v.MountPath,
		ReadOnly:  v.ReadOnly,
	}
	if v.SubPath != "" {
		vm.SubPath = v.SubPath
	}
	return vm
}

func buildVolume(v types.VolumeConfig) corev1.Volume {
	vol := corev1.Volume{Name: v.Name}
	switch {
	case v.Secret != "":
		vol.VolumeSource = corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{SecretName: v.Secret},
		}
	case v.ConfigMap != "":
		vol.VolumeSource = corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: v.ConfigMap},
			},
		}
	case v.EmptyDir:
		vol.VolumeSource = corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		}
	case v.PVC != "":
		vol.VolumeSource = corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: v.PVC,
				ReadOnly:  v.ReadOnly,
			},
		}
	case v.HostPath != "":
		vol.VolumeSource = corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: v.HostPath,
			},
		}
	}
	return vol
}

func buildVolumes(vols []types.VolumeConfig) []corev1.Volume {
	var result []corev1.Volume
	for _, v := range vols {
		result = append(result, buildVolume(v))
	}
	return result
}

func buildSecurityContext(sc *types.SecurityContextConfig) *corev1.SecurityContext {
	ctx := &corev1.SecurityContext{}
	if sc.RunAsUser != nil {
		ctx.RunAsUser = sc.RunAsUser
	}
	if sc.RunAsGroup != nil {
		ctx.RunAsGroup = sc.RunAsGroup
	}
	if sc.RunAsNonRoot != nil {
		ctx.RunAsNonRoot = sc.RunAsNonRoot
	}
	if sc.ReadOnlyRoot != nil {
		ctx.ReadOnlyRootFilesystem = sc.ReadOnlyRoot
	}
	return ctx
}

func applyProbeTuning(probe *corev1.Probe, p types.PortConfig) {
	if p.InitialDelay != nil {
		probe.InitialDelaySeconds = *p.InitialDelay
	}
	if p.Period != nil {
		probe.PeriodSeconds = *p.Period
	}
	if p.Timeout != nil {
		probe.TimeoutSeconds = *p.Timeout
	}
	if p.Failures != nil {
		probe.FailureThreshold = *p.Failures
	}
	if p.Successes != nil {
		probe.SuccessThreshold = *p.Successes
	}
}

func volumeExists(volumes []corev1.Volume, name string) bool {
	for _, v := range volumes {
		if v.Name == name {
			return true
		}
	}
	return false
}

// keep intstr import used
var _ = intstr.FromInt32
