package generator

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/gsid-nl/kdef/internal/types"
)

func TestResolveServiceAccount(t *testing.T) {
	sas := []types.ServiceAccountConfig{
		{Name: "default", ImagePullSecrets: []string{"shared"}},
		{Name: "default", Namespace: "monitoring", ImagePullSecrets: []string{"mon-reg"}},
		{Name: "alloy", Namespace: "logs"},
	}

	cases := []struct {
		name, ns       string
		wantOK         bool
		wantPullSecret string
	}{
		{"default", "production", true, "shared"},
		{"default", "monitoring", true, "mon-reg"},
		{"alloy", "logs", true, ""},
		{"alloy", "production", false, ""},
		{"missing", "production", false, ""},
	}

	for _, tc := range cases {
		got, ok := resolveServiceAccount(sas, tc.name, tc.ns)
		if ok != tc.wantOK {
			t.Errorf("%s/%s: ok=%v, want %v", tc.name, tc.ns, ok, tc.wantOK)
			continue
		}
		if !ok {
			continue
		}
		var firstPull string
		if len(got.ImagePullSecrets) > 0 {
			firstPull = got.ImagePullSecrets[0]
		}
		if firstPull != tc.wantPullSecret {
			t.Errorf("%s/%s: imagePullSecret=%q, want %q", tc.name, tc.ns, firstPull, tc.wantPullSecret)
		}
	}
}

func TestGenerate_ScopedServiceAccountsEmitCorrectPullSecrets(t *testing.T) {
	config := &types.KdefConfig{
		Namespaces: []string{"production", "monitoring"},
		ServiceAccounts: []types.ServiceAccountConfig{
			{Name: "default", ImagePullSecrets: []string{"shared"}},
			{Name: "default", Namespace: "monitoring", ImagePullSecrets: []string{"mon-reg"}},
		},
		Deployments: []types.DeploymentConfig{
			{Name: "api", Namespace: "production", ServiceAccountName: "default",
				Containers: []types.ContainerConfig{{Name: "api", Image: "x"}}},
			{Name: "node-exp", Namespace: "monitoring", ServiceAccountName: "default",
				Containers: []types.ContainerConfig{{Name: "x", Image: "x"}}},
		},
	}

	result := Generate(config)

	prodSA := saFromResult(t, result, "sa-default-production")
	if got := pullSecretNames(prodSA); len(got) != 1 || got[0] != "shared" {
		t.Errorf("production default SA imagePullSecrets=%v, want [shared]", got)
	}

	monSA := saFromResult(t, result, "sa-default-monitoring")
	if got := pullSecretNames(monSA); len(got) != 1 || got[0] != "mon-reg" {
		t.Errorf("monitoring default SA imagePullSecrets=%v, want [mon-reg]", got)
	}
}

func saFromResult(t *testing.T, result map[string][]Manifest, key string) *corev1.ServiceAccount {
	t.Helper()
	manifests, ok := result[key]
	if !ok || len(manifests) == 0 {
		t.Fatalf("missing manifest %q", key)
	}
	sa, ok := manifests[0].Object.(*corev1.ServiceAccount)
	if !ok {
		t.Fatalf("manifest %q is not a ServiceAccount: %T", key, manifests[0].Object)
	}
	return sa
}

func pullSecretNames(sa *corev1.ServiceAccount) []string {
	var names []string
	for _, s := range sa.ImagePullSecrets {
		names = append(names, s.Name)
	}
	return names
}
