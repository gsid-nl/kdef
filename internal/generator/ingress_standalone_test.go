package generator

import (
	"testing"

	networkingv1 "k8s.io/api/networking/v1"

	"github.com/gsid-nl/kdef/internal/types"
)

func TestGenerateStandaloneIngress(t *testing.T) {
	ing := types.IngressConfig{
		Name:        "cms-gsid-nl",
		Namespace:   "cms",
		ServiceName: "cms-public-svc",
		Port:        80,
		Hosts:       []string{"gsid.nl", "www.gsid.nl"},
		TLS:         true,
		Issuer:      "letsencrypt-production",
		Class:       "haproxy",
	}

	manifests := GenerateStandaloneIngress(ing, &types.IngressDefaults{})
	if len(manifests) != 2 {
		t.Fatalf("expected an Ingress and a Certificate, got %d manifests", len(manifests))
	}

	obj, ok := manifests[0].Object.(*networkingv1.Ingress)
	if !ok {
		t.Fatalf("first manifest is %T, want *networkingv1.Ingress", manifests[0].Object)
	}
	if obj.Name != "cms-gsid-nl" || obj.Namespace != "cms" {
		t.Errorf("unexpected metadata: %s/%s", obj.Namespace, obj.Name)
	}
	if got := obj.Labels["app.kubernetes.io/name"]; got != "cms-public-svc" {
		t.Errorf("app label should follow the backend service, got %q", got)
	}
	if *obj.Spec.IngressClassName != "haproxy" {
		t.Errorf("unexpected ingress class %q", *obj.Spec.IngressClassName)
	}
	if len(obj.Spec.Rules) != 2 {
		t.Fatalf("expected one rule per host, got %d", len(obj.Spec.Rules))
	}
	backend := obj.Spec.Rules[0].HTTP.Paths[0].Backend.Service
	if backend.Name != "cms-public-svc" || backend.Port.Number != 80 {
		t.Errorf("unexpected backend %s:%d", backend.Name, backend.Port.Number)
	}
	if len(obj.Spec.TLS) != 1 || obj.Spec.TLS[0].SecretName != "gsid-nl-tls" {
		t.Errorf("unexpected TLS block: %+v", obj.Spec.TLS)
	}

	cert := manifests[1].Object
	if got := cert.(interface{ GetName() string }).GetName(); got != "gsid-nl-tls" {
		t.Errorf("unexpected certificate name %q", got)
	}
}

func TestGenerateStandaloneIngress_GatewayMode(t *testing.T) {
	ing := types.IngressConfig{
		Name:        "cms-gsid-nl",
		Namespace:   "cms",
		ServiceName: "cms-public-svc",
		Port:        80,
		Host:        "gsid.nl",
	}

	manifests := GenerateStandaloneIngress(ing, &types.IngressDefaults{
		Mode:    "gateway",
		Gateway: "public-gateway",
	})
	if len(manifests) != 1 {
		t.Fatalf("expected a single HTTPRoute, got %d manifests", len(manifests))
	}
	if _, ok := manifests[0].Object.(*networkingv1.Ingress); ok {
		t.Fatal("gateway mode should not emit a classic Ingress")
	}
}
