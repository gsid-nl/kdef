package generator

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/gsid-nl/kdef/internal/types"
	"github.com/gsid-nl/kdef/internal/version"
)

const defaultIssuer = "letsencrypt-production"

// GenerateCertificate creates a cert-manager Certificate for an app.
// Returns nil if the app has no ingress or TLS is not enabled.
func GenerateCertificate(app types.AppConfig) *unstructured.Unstructured {
	if app.Ingress == nil || !app.Ingress.TLS {
		return nil
	}

	// If an existing TLS secret is specified, don't generate a Certificate
	if app.Ingress.TLSSecret != "" {
		return nil
	}

	hosts := allHosts(app.Ingress)
	if len(hosts) == 0 {
		return nil
	}

	issuer := app.Ingress.Issuer
	if issuer == "" {
		issuer = defaultIssuer
	}

	secretName := fmt.Sprintf("%s-tls", strings.ReplaceAll(hosts[0], ".", "-"))

	// Build dnsNames as []interface{} for unstructured
	dnsNames := make([]interface{}, len(hosts))
	for i, h := range hosts {
		dnsNames[i] = h
	}

	cert := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Certificate",
			"metadata": map[string]interface{}{
				"name": secretName,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       app.Name,
					"app.kubernetes.io/managed-by": "kdef",
					"kdef.dev/version":         version.Version,
				},
			},
			"spec": map[string]interface{}{
				"secretName": secretName,
				"dnsNames":   dnsNames,
				"issuerRef": map[string]interface{}{
					"name": issuer,
					"kind": "ClusterIssuer",
				},
			},
		},
	}

	if app.Namespace != "" {
		cert.SetNamespace(app.Namespace)
	}

	return cert
}
