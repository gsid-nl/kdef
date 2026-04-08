package generator

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/gsid-nl/kdef/internal/types"
	"github.com/gsid-nl/kdef/internal/version"
)

// GenerateSealedSecret creates a Bitnami SealedSecret from a SealedSecretConfig.
func GenerateSealedSecret(ss types.SealedSecretConfig) *unstructured.Unstructured {
	encryptedData := make(map[string]interface{}, len(ss.Data))
	for k, v := range ss.Data {
		encryptedData[k] = v
	}

	secretType := ss.Type
	if secretType == "" {
		secretType = "Opaque"
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "bitnami.com/v1alpha1",
			"kind":       "SealedSecret",
			"metadata": map[string]interface{}{
				"name": ss.Name,
				"labels": map[string]interface{}{
					"app.kubernetes.io/managed-by": "kdef",
					"kdef.gsid.nl/version":         version.Version,
				},
			},
			"spec": map[string]interface{}{
				"encryptedData": encryptedData,
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": ss.Name,
						"labels": map[string]interface{}{
							"app.kubernetes.io/managed-by": "kdef",
							"kdef.gsid.nl/version":         version.Version,
						},
					},
					"type": secretType,
				},
			},
		},
	}

	if ss.Namespace != "" {
		obj.SetNamespace(ss.Namespace)
		// Also set namespace in the template metadata
		template := obj.Object["spec"].(map[string]interface{})["template"].(map[string]interface{})
		templateMeta := template["metadata"].(map[string]interface{})
		templateMeta["namespace"] = ss.Namespace
	}

	return obj
}
