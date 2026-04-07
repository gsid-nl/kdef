package generator

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/gsid-nl/kdef/internal/types"
)

// GenerateDeploymentV2 creates all manifests from a DeploymentConfig.
func GenerateDeploymentV2(dep types.DeploymentConfig) []Manifest {
	var manifests []Manifest

	// Labels
	labels := map[string]string{
		"app.kubernetes.io/name": dep.Name,
	}
	if len(dep.Labels) > 0 {
		labels = make(map[string]string)
		for k, v := range dep.Labels {
			labels[k] = v
		}
	}
	kdefLabels(labels)

	// Selector
	selector := map[string]string{
		"app.kubernetes.io/name": dep.Name,
	}
	if len(dep.Selector) > 0 {
		selector = dep.Selector
	}

	replicas := dep.Replicas

	// Build containers
	var containers []corev1.Container
	for _, c := range dep.Containers {
		containers = append(containers, buildContainerV2(c))
	}

	// Build init containers
	var initContainers []corev1.Container
	for _, ic := range dep.InitContainers {
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
		for _, e := range ic.Env {
			initC.Env = append(initC.Env, buildEnvVar(e))
		}
		for _, ef := range ic.EnvFrom {
			initC.EnvFrom = append(initC.EnvFrom, buildEnvFromSource(ef))
		}
		// Init container volume mounts — reference volumes by name from deployment volumes
		for _, volName := range ic.VolumeMounts {
			for _, v := range dep.Volumes {
				if v.Name == volName {
					initC.VolumeMounts = append(initC.VolumeMounts, buildVolumeMount(v))
				}
			}
		}
		if ic.SecurityContext != nil {
			initC.SecurityContext = buildSecurityContext(ic.SecurityContext)
		}
		initContainers = append(initContainers, initC)
	}

	// Pod-level volumes (from deployment + all container volumes, deduplicated)
	volumes := buildVolumes(dep.Volumes)
	for _, c := range dep.Containers {
		for _, cv := range c.Volumes {
			if !volumeExists(volumes, cv.Name) {
				volumes = append(volumes, buildVolume(cv))
			}
		}
	}

	// ImagePullSecrets
	var imagePullSecrets []corev1.LocalObjectReference
	for _, s := range dep.ImagePullSecrets {
		imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{Name: s})
	}

	// Pod spec
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
	if dep.ServiceAccountName != "" {
		spec.ServiceAccountName = dep.ServiceAccountName
	}
	if dep.SecurityContext != nil && dep.SecurityContext.FSGroup != nil {
		spec.SecurityContext = &corev1.PodSecurityContext{
			FSGroup: dep.SecurityContext.FSGroup,
		}
	}

	// Deployment
	k8sDep := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      dep.Name,
			Namespace: dep.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: selector,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: spec,
			},
		},
	}
	manifests = append(manifests, Manifest{Object: k8sDep, Raw: dep.Raw})

	// Service — generated if any container has ports and service block exists (or implicit)
	if dep.Service != nil {
		svcName := dep.Name
		if dep.Service.Name != "" {
			svcName = dep.Service.Name
		}

		var svcPorts []corev1.ServicePort
		if len(dep.Service.Ports) > 0 {
			// Explicit port definitions
			for _, sp := range dep.Service.Ports {
				svcPorts = append(svcPorts, corev1.ServicePort{
					Name:       sp.Name,
					Port:       sp.Number,
					TargetPort: intstr.FromInt32(sp.Target),
					Protocol:   corev1.ProtocolTCP,
				})
			}
		} else {
			// Fallback: collect all ports from all containers
			for _, c := range dep.Containers {
				for _, p := range c.Ports {
					svcPorts = append(svcPorts, corev1.ServicePort{
						Name:       p.Name,
						Port:       p.Number,
						TargetPort: intstr.FromInt32(p.Number),
						Protocol:   corev1.ProtocolTCP,
					})
				}
			}
		}

		svcType := corev1.ServiceTypeClusterIP
		if dep.Service.Type == "NodePort" {
			svcType = corev1.ServiceTypeNodePort
		} else if dep.Service.Type == "LoadBalancer" {
			svcType = corev1.ServiceTypeLoadBalancer
		}

		svc := &corev1.Service{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Service",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      svcName,
				Namespace: dep.Namespace,
				Labels:    kdefLabels(map[string]string{"app.kubernetes.io/name": dep.Name}),
			},
			Spec: corev1.ServiceSpec{
				Type:     svcType,
				Selector: selector,
				Ports:    svcPorts,
			},
		}
		manifests = append(manifests, Manifest{Object: svc})
	}

	// Ingress
	if dep.Ingress != nil {
		// Convert to AppConfig-compatible for the ingress generator
		// (reuse existing logic)
		appCompat := types.AppConfig{
			Name:      dep.Name,
			Namespace: dep.Namespace,
			Labels:    dep.Labels,
			Selector:  dep.Selector,
			Ingress:   dep.Ingress,
		}
		// Collect all ports from all containers for the ingress
		for _, c := range dep.Containers {
			appCompat.Ports = append(appCompat.Ports, c.Ports...)
		}
		if ing := GenerateIngress(appCompat); ing != nil {
			manifests = append(manifests, Manifest{Object: ing})
		}
		if cert := GenerateCertificate(appCompat); cert != nil {
			manifests = append(manifests, Manifest{Object: cert})
		}
	}

	// HPA
	if dep.Autoscale != nil {
		appCompat := types.AppConfig{
			Name:      dep.Name,
			Namespace: dep.Namespace,
			Selector:  dep.Selector,
			Autoscale: dep.Autoscale,
		}
		if hpa := GenerateHPA(appCompat); hpa != nil {
			manifests = append(manifests, Manifest{Object: hpa})
		}
	}

	return manifests
}

func buildContainerV2(c types.ContainerConfig) corev1.Container {
	container := corev1.Container{
		Name:  c.Name,
		Image: c.Image,
	}

	if c.ImagePullPolicy != "" {
		container.ImagePullPolicy = corev1.PullPolicy(c.ImagePullPolicy)
	}
	if len(c.Command) > 0 {
		container.Command = c.Command
	}

	for _, p := range c.Ports {
		container.Ports = append(container.Ports, corev1.ContainerPort{
			Name:          p.Name,
			ContainerPort: p.Number,
			Protocol:      corev1.ProtocolTCP,
		})
	}

	for _, e := range c.Env {
		container.Env = append(container.Env, buildEnvVar(e))
	}
	for _, ef := range c.EnvFrom {
		container.EnvFrom = append(container.EnvFrom, buildEnvFromSource(ef))
	}

	// Probes
	for _, p := range c.Ports {
		if (p.Health != "" || p.TCPHealth) && container.LivenessProbe == nil {
			probe := &corev1.Probe{}
			if p.Health != "" {
				probe.ProbeHandler = corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{Path: p.Health, Port: intstr.FromInt32(p.Number)},
				}
			} else {
				probe.ProbeHandler = corev1.ProbeHandler{
					TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(p.Number)},
				}
			}
			applyProbeTuning(probe, p)
			container.LivenessProbe = probe
		}
		if (p.Ready != "" || p.TCPReady) && container.ReadinessProbe == nil {
			probe := &corev1.Probe{}
			if p.Ready != "" {
				probe.ProbeHandler = corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{Path: p.Ready, Port: intstr.FromInt32(p.Number)},
				}
			} else {
				probe.ProbeHandler = corev1.ProbeHandler{
					TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(p.Number)},
				}
			}
			applyProbeTuning(probe, p)
			container.ReadinessProbe = probe
		}
	}

	if c.Resources != nil {
		container.Resources = buildResources(c.Resources)
	}

	for _, v := range c.Volumes {
		container.VolumeMounts = append(container.VolumeMounts, buildVolumeMount(v))
	}

	if c.SecurityContext != nil {
		container.SecurityContext = buildSecurityContext(c.SecurityContext)
	}

	return container
}

func hasPorts(dep types.DeploymentConfig) bool {
	for _, c := range dep.Containers {
		if len(c.Ports) > 0 {
			return true
		}
	}
	return false
}
