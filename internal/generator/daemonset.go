package generator

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gsid-nl/kdef/internal/types"
)

// GenerateDaemonSet creates all manifests from a DaemonSetConfig.
func GenerateDaemonSet(ds types.DaemonSetConfig) []Manifest {
	var manifests []Manifest

	labels := map[string]string{
		"app.kubernetes.io/name": ds.Name,
	}
	if len(ds.Labels) > 0 {
		labels = make(map[string]string)
		for k, v := range ds.Labels {
			labels[k] = v
		}
	}
	kdefLabels(labels)

	selector := map[string]string{
		"app.kubernetes.io/name": ds.Name,
	}
	if len(ds.Selector) > 0 {
		selector = ds.Selector
	}

	spec := buildPodSpec(PodTemplateInput{
		Containers:         ds.Containers,
		InitContainers:     ds.InitContainers,
		Volumes:            ds.Volumes,
		ImagePullSecrets:   ds.ImagePullSecrets,
		ServiceAccountName: ds.ServiceAccountName,
		SecurityContext:    ds.SecurityContext,
		NodeSelector:       ds.NodeSelector,
		Tolerations:        ds.Tolerations,
	}, nil)

	k8sDS := &appsv1.DaemonSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "DaemonSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ds.Name,
			Namespace: ds.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DaemonSetSpec{
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
	manifests = append(manifests, Manifest{Object: k8sDS, Raw: ds.Raw})

	if ds.Service != nil {
		svc := buildWorkloadService(ds.Name, ds.Namespace, ds.Service, ds.Containers, selector, false)
		manifests = append(manifests, Manifest{Object: svc})
	}

	return manifests
}
