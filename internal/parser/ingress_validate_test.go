package parser

import (
	"strings"
	"testing"

	"github.com/gsid-nl/kdef/internal/types"
)

func TestValidateIngresses_OK(t *testing.T) {
	cfg := &types.KdefConfig{
		Deployments: []types.DeploymentConfig{
			{
				Name:      "web",
				Namespace: "prod",
				Ingresses: []types.IngressConfig{
					{Host: "www.example.com", TLS: true},
					{Host: "example.com", Annotations: map[string]string{
						"nginx.ingress.kubernetes.io/permanent-redirect": "https://www.example.com$request_uri",
					}},
				},
			},
		},
	}
	if err := validateIngresses(cfg); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateIngresses_DuplicateResourceName_WithinWorkload(t *testing.T) {
	cfg := &types.KdefConfig{
		Deployments: []types.DeploymentConfig{
			{
				Name:      "web",
				Namespace: "prod",
				Ingresses: []types.IngressConfig{
					{Name: "shared", Host: "a.example.com"},
					{Name: "shared", Host: "b.example.com"},
				},
			},
		},
	}
	err := validateIngresses(cfg)
	if err == nil {
		t.Fatal("expected error for duplicate explicit names within workload")
	}
	if !strings.Contains(err.Error(), "shared") {
		t.Errorf("error message should reference the duplicate name, got: %v", err)
	}
}

func TestValidateIngresses_DuplicateResourceName_AutoSuffixCollision(t *testing.T) {
	// Auto-suffix on block #2 produces "web-2"; an explicit name="web-2"
	// somewhere else in the same namespace must be rejected.
	cfg := &types.KdefConfig{
		Deployments: []types.DeploymentConfig{
			{
				Name:      "web",
				Namespace: "prod",
				Ingresses: []types.IngressConfig{
					{Host: "a.example.com"},
					{Host: "b.example.com"}, // auto: "web-2"
				},
			},
			{
				Name:      "other",
				Namespace: "prod",
				Ingresses: []types.IngressConfig{
					{Name: "web-2", Host: "c.example.com"},
				},
			},
		},
	}
	err := validateIngresses(cfg)
	if err == nil {
		t.Fatal("expected error for cross-workload resource name collision")
	}
	if !strings.Contains(err.Error(), "web-2") {
		t.Errorf("error message should reference the colliding name, got: %v", err)
	}
}

func TestValidateIngresses_CrossNamespace_SameName_OK(t *testing.T) {
	// Same K8s name in different namespaces is fine.
	cfg := &types.KdefConfig{
		Deployments: []types.DeploymentConfig{
			{
				Name:      "web",
				Namespace: "prod",
				Ingresses: []types.IngressConfig{
					{Host: "a.example.com"},
				},
			},
			{
				Name:      "web",
				Namespace: "staging",
				Ingresses: []types.IngressConfig{
					{Host: "b.example.com"},
				},
			},
		},
	}
	if err := validateIngresses(cfg); err != nil {
		t.Fatalf("expected no error across namespaces, got: %v", err)
	}
}

func TestValidateIngresses_DuplicateCertSecret(t *testing.T) {
	// Both TLS ingresses share first host "shared.example.com" → same
	// derived Certificate secret "shared-example-com-tls" in same namespace.
	cfg := &types.KdefConfig{
		Deployments: []types.DeploymentConfig{
			{
				Name:      "web",
				Namespace: "prod",
				Ingresses: []types.IngressConfig{
					{Name: "web", Host: "shared.example.com", TLS: true},
					{Name: "web-extra", Host: "shared.example.com", TLS: true},
				},
			},
		},
	}
	err := validateIngresses(cfg)
	if err == nil {
		t.Fatal("expected error for duplicate Certificate secret name")
	}
	if !strings.Contains(err.Error(), "shared-example-com-tls") {
		t.Errorf("error message should name the colliding secret, got: %v", err)
	}
}

func TestValidateIngresses_CertSecret_TLSSecretOverrideAvoidsConflict(t *testing.T) {
	// One block sets `tls_secret`, so no Certificate is generated for it
	// → no conflict with the other block that *does* derive the same secret.
	cfg := &types.KdefConfig{
		Deployments: []types.DeploymentConfig{
			{
				Name:      "web",
				Namespace: "prod",
				Ingresses: []types.IngressConfig{
					{Name: "web", Host: "shared.example.com", TLS: true},
					{Name: "web-extra", Host: "shared.example.com", TLS: true, TLSSecret: "byo-secret"},
				},
			},
		},
	}
	if err := validateIngresses(cfg); err != nil {
		t.Fatalf("expected no error when one ingress uses tls_secret, got: %v", err)
	}
}
