package generator

import (
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gsid-nl/kdef/internal/types"
)

// GenerateIngress creates an Ingress for an app. Returns nil if no ingress block is defined.
func GenerateIngress(app types.AppConfig) *networkingv1.Ingress {
	if app.Ingress == nil {
		return nil
	}

	hosts := app.Ingress.AllHosts()
	if len(hosts) == 0 {
		return nil
	}

	pathType := networkingv1.PathTypePrefix
	ingressClassName := "nginx"

	// Build paths from ports
	// Backend service name — defaults to app name
	svcName := app.Name
	if app.Ingress.ServiceName != "" {
		svcName = app.Ingress.ServiceName
	}

	// Determine backend port: explicit ingress port > first app port > 80
	var backendPort int32 = 80
	if app.Ingress.Port > 0 {
		backendPort = app.Ingress.Port
	} else if len(app.Ports) > 0 {
		backendPort = app.Ports[0].Number
	}

	paths := []networkingv1.HTTPIngressPath{
		{
			Path:     "/",
			PathType: &pathType,
			Backend: networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: svcName,
					Port: networkingv1.ServiceBackendPort{
						Number: backendPort,
					},
				},
			},
		},
	}

	// One rule per host
	var rules []networkingv1.IngressRule
	for _, host := range hosts {
		rules = append(rules, networkingv1.IngressRule{
			Host: host,
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: paths,
				},
			},
		})
	}

	ingress := &networkingv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "networking.k8s.io/v1",
			Kind:       "Ingress",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressName(app),
			Namespace: app.Namespace,
			Labels: kdefLabels(map[string]string{
				"app.kubernetes.io/name": app.Name,
			}),
			Annotations: app.Ingress.Annotations,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules:            rules,
		},
	}

	if app.Ingress.TLS {
		secretName := app.Ingress.TLSSecret
		if secretName == "" {
			secretName = app.Ingress.CertificateSecretName()
		}
		ingress.Spec.TLS = []networkingv1.IngressTLS{
			{
				Hosts:      hosts,
				SecretName: secretName,
			},
		}
	}

	return ingress
}

func ingressName(app types.AppConfig) string {
	if app.Ingress.Name != "" {
		return app.Ingress.Name
	}
	return app.Name
}
