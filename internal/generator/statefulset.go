package generator

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gsid-nl/kdef/internal/types"
)

// GenerateStatefulSet creates all manifests from a StatefulSetConfig.
func GenerateStatefulSet(sts types.StatefulSetConfig, ingressDefaults *types.IngressDefaults) []Manifest {
	var manifests []Manifest

	labels := map[string]string{
		"app.kubernetes.io/name": sts.Name,
	}
	if len(sts.Labels) > 0 {
		labels = make(map[string]string)
		for k, v := range sts.Labels {
			labels[k] = v
		}
	}
	kdefLabels(labels)

	selector := map[string]string{
		"app.kubernetes.io/name": sts.Name,
	}
	if len(sts.Selector) > 0 {
		selector = sts.Selector
	}

	replicas := sts.Replicas

	claimNames := make(map[string]bool)
	for _, vc := range sts.VolumeClaims {
		claimNames[vc.Name] = true
	}

	spec := buildPodSpec(PodTemplateInput{
		Containers:         sts.Containers,
		InitContainers:     sts.InitContainers,
		Volumes:            sts.Volumes,
		ImagePullSecrets:   sts.ImagePullSecrets,
		ServiceAccountName: sts.ServiceAccountName,
		SecurityContext:    sts.SecurityContext,
		NodeSelector:       sts.NodeSelector,
		Tolerations:        sts.Tolerations,
		HostNetwork:        sts.HostNetwork,
		HostPID:            sts.HostPID,
		HostIPC:            sts.HostIPC,
		DNSPolicy:          sts.DNSPolicy,
	}, claimNames)

	serviceName := sts.ServiceName
	if serviceName == "" {
		serviceName = sts.Name
	}

	stsSpec := appsv1.StatefulSetSpec{
		Replicas:    &replicas,
		ServiceName: serviceName,
		Selector: &metav1.LabelSelector{
			MatchLabels: selector,
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: labels,
			},
			Spec: spec,
		},
	}

	if sts.PodManagementPolicy != "" {
		stsSpec.PodManagementPolicy = appsv1.PodManagementPolicyType(sts.PodManagementPolicy)
	}

	for _, vc := range sts.VolumeClaims {
		stsSpec.VolumeClaimTemplates = append(stsSpec.VolumeClaimTemplates, buildVolumeClaimTemplate(vc))
	}

	k8sSTS := &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      sts.Name,
			Namespace: sts.Namespace,
			Labels:    labels,
		},
		Spec: stsSpec,
	}
	manifests = append(manifests, Manifest{Object: k8sSTS, Raw: sts.Raw})

	if sts.Service != nil {
		// If the user-declared service name matches the STS's serviceName and
		// no explicit type is set, emit a headless (ClusterIP=None) Service.
		svcName := sts.Name
		if sts.Service.Name != "" {
			svcName = sts.Service.Name
		}
		headless := svcName == serviceName && sts.Service.Type == ""
		svc := buildWorkloadService(sts.Name, sts.Namespace, sts.Service, sts.Containers, selector, headless)
		manifests = append(manifests, Manifest{Object: svc})
	}

	if len(sts.Ingresses) > 0 {
		var ports []types.PortConfig
		for _, c := range sts.Containers {
			ports = append(ports, c.Ports...)
		}
		for i := range sts.Ingresses {
			ingCfg := sts.Ingresses[i]
			ingCfg.Name = ingCfg.ResourceName(sts.Name, i)
			appCompat := types.AppConfig{
				Name:      sts.Name,
				Namespace: sts.Namespace,
				Labels:    sts.Labels,
				Selector:  sts.Selector,
				Ports:     ports,
				Ingress:   &ingCfg,
			}
			if ingressDefaults != nil {
				appCompat.IngressMode = ingressDefaults.Mode
				appCompat.IngressGateway = ingressDefaults.Gateway
				appCompat.IngressGatewayNS = ingressDefaults.GatewayNamespace
			}
			if appCompat.IngressMode == "gateway" {
				if route := GenerateHTTPRoute(appCompat); route != nil {
					manifests = append(manifests, Manifest{Object: route})
				}
			} else {
				if ing := GenerateIngress(appCompat); ing != nil {
					manifests = append(manifests, Manifest{Object: ing})
				}
			}
			if cert := GenerateCertificate(appCompat); cert != nil {
				manifests = append(manifests, Manifest{Object: cert})
			}
		}
	}

	return manifests
}

func buildVolumeClaimTemplate(vc types.VolumeClaimTemplate) corev1.PersistentVolumeClaim {
	accessModes := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	if len(vc.AccessModes) > 0 {
		accessModes = nil
		for _, m := range vc.AccessModes {
			accessModes = append(accessModes, corev1.PersistentVolumeAccessMode(m))
		}
	}

	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: vc.Name,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(vc.Storage),
				},
			},
		},
	}
	if vc.StorageClass != "" {
		sc := vc.StorageClass
		pvc.Spec.StorageClassName = &sc
	}
	return pvc
}
