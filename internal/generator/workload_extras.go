package generator

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/gsid-nl/kdef/internal/types"
)

// buildWorkloadService builds a Service manifest for a workload. The workload
// name is used as the default service name and as the app.kubernetes.io/name
// label. `clusterIPNone` emits a headless Service (typical for StatefulSets).
func buildWorkloadService(
	name, namespace string,
	svc *types.ServiceConfig,
	containers []types.ContainerConfig,
	selector map[string]string,
	clusterIPNone bool,
) *corev1.Service {
	svcName := name
	if svc.Name != "" {
		svcName = svc.Name
	}

	var svcPorts []corev1.ServicePort
	if len(svc.Ports) > 0 {
		for _, sp := range svc.Ports {
			svcPorts = append(svcPorts, corev1.ServicePort{
				Name:       sp.Name,
				Port:       sp.Number,
				TargetPort: intstr.FromInt32(sp.Target),
				Protocol:   corev1.ProtocolTCP,
			})
		}
	} else {
		for _, c := range containers {
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
	switch svc.Type {
	case "NodePort":
		svcType = corev1.ServiceTypeNodePort
	case "LoadBalancer":
		svcType = corev1.ServiceTypeLoadBalancer
	}

	spec := corev1.ServiceSpec{
		Type:     svcType,
		Selector: selector,
		Ports:    svcPorts,
	}
	if clusterIPNone {
		spec.ClusterIP = "None"
	}

	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: namespace,
			Labels:    kdefLabels(map[string]string{"app.kubernetes.io/name": name}),
		},
		Spec: spec,
	}
}
