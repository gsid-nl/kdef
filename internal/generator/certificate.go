package generator

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/gsid-nl/kdef/internal/types"
	"github.com/gsid-nl/kdef/internal/version"
)

const defaultIssuer = "letsencrypt-production"

// GenerateCertificate creates a cert-manager Certificate for an app.
// Returns nil if the app has no ingress or TLS is not enabled.
func GenerateCertificate(app types.AppConfig) *unstructured.Unstructured {
	if app.Ingress == nil {
		return nil
	}

	secretName := app.Ingress.CertificateSecretName()
	if secretName == "" {
		// TLS disabled, explicit tls_secret, or no hosts — no Certificate.
		return nil
	}

	hosts := app.Ingress.AllHosts()

	issuer := app.Ingress.Issuer
	if issuer == "" {
		issuer = defaultIssuer
	}

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
