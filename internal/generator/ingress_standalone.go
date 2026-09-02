package generator

import (
	"github.com/gsid-nl/kdef/internal/types"
)

// GenerateStandaloneIngress produces the manifests for a top-level `ingress`
// block: an Ingress (or HTTPRoute in gateway mode) plus a cert-manager
// Certificate when TLS is on without an explicit tls_secret.
//
// Unlike a nested block there is no workload to inherit from, so the parser
// requires service_name and port; the backend is never guessed here.
func GenerateStandaloneIngress(ing types.IngressConfig, ingressDefaults *types.IngressDefaults) []Manifest {
	// app.kubernetes.io/name follows the backend service so a standalone
	// ingress still groups with the workload it fronts under label selectors.
	labelName := ing.ServiceName
	if labelName == "" {
		labelName = ing.Name
	}

	appCompat := types.AppConfig{
		Name:      labelName,
		Namespace: ing.Namespace,
		Ingress:   &ing,
	}
	if ingressDefaults != nil {
		appCompat.IngressMode = ingressDefaults.Mode
		appCompat.IngressGateway = ingressDefaults.Gateway
		appCompat.IngressGatewayNS = ingressDefaults.GatewayNamespace
	}

	var manifests []Manifest
	if appCompat.IngressMode == "gateway" {
		if route := GenerateHTTPRoute(appCompat); route != nil {
			manifests = append(manifests, Manifest{Object: route})
		}
	} else {
		if ingress := GenerateIngress(appCompat); ingress != nil {
			manifests = append(manifests, Manifest{Object: ingress})
		}
	}
	if cert := GenerateCertificate(appCompat); cert != nil {
		manifests = append(manifests, Manifest{Object: cert})
	}
	return manifests
}
