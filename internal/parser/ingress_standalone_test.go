package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsid-nl/kdef/internal/types"
)

// writeRootProject lays out a minimal root.kdef project with a single
// sub-project directory "cms" mapped to the "cms" namespace.
func writeRootProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cms"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const rootWithIngressDefaults = `
namespaces = ["cms"]

ingress_defaults {
  tls    = true
  issuer = "letsencrypt-production"
  class  = "haproxy"
  annotations = {
    "haproxy.org" = {
      "client-body-buffer-size" = "50m"
    }
  }
}

deployments = {
  "cms" = {
    path      = "cms"
    namespace = "cms"
  }
}
`

func TestStandaloneIngress_DefaultsAndNamespaceInjection(t *testing.T) {
	dir := writeRootProject(t, map[string]string{
		"root.kdef": rootWithIngressDefaults,
		"cms/cms.kdef": `
deployment "cms-public" {
  container "cms" {
    image = "nginx"
    port "80" "http" {}
  }
  service {
    name = "cms-public-svc"
    port "80" "http" {}
  }
}

ingress "cms-gsid-nl" {
  service_name = "cms-public-svc"
  port         = 80
  hosts        = ["gsid.nl", "www.gsid.nl"]
  annotations = {
    "haproxy.org" = {
      "client-body-buffer-size" = "100m"
    }
  }
}
`,
	})

	config, err := Load(dir, nil)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	if len(config.Ingresses) != 1 {
		t.Fatalf("expected 1 standalone ingress, got %d", len(config.Ingresses))
	}
	ing := config.Ingresses[0]

	if ing.Name != "cms-gsid-nl" {
		t.Errorf("name should come from the block label, got %q", ing.Name)
	}
	if ing.Namespace != "cms" {
		t.Errorf("namespace should be injected from root.kdef, got %q", ing.Namespace)
	}
	if !ing.TLS {
		t.Error("tls should be inherited from ingress_defaults")
	}
	if ing.Issuer != "letsencrypt-production" {
		t.Errorf("issuer should be inherited from ingress_defaults, got %q", ing.Issuer)
	}
	if ing.Class != "haproxy" {
		t.Errorf("class should be inherited from ingress_defaults, got %q", ing.Class)
	}
	if got := ing.Annotations["haproxy.org/client-body-buffer-size"]; got != "100m" {
		t.Errorf("block annotation should win over the default, got %q", got)
	}
	if ing.ServiceName != "cms-public-svc" || ing.Port != 80 {
		t.Errorf("backend not parsed: service=%q port=%d", ing.ServiceName, ing.Port)
	}
	if got := ing.CertificateSecretName(); got != "gsid-nl-tls" {
		t.Errorf("cert secret should derive from the first host, got %q", got)
	}
}

func TestStandaloneIngress_ForLoopOverValues(t *testing.T) {
	dir := writeRootProject(t, map[string]string{
		"root.kdef": rootWithIngressDefaults,
		"cms/cms-ingresses.kdef": `
for "site" "var.sites" {
  ingress "cms" {
    name         = "cms-${site.name}"
    service_name = "cms-public-svc"
    port         = 80
    hosts        = site.hosts
  }
}
`,
	})

	valuesFile := filepath.Join(dir, "sites.json")
	if err := os.WriteFile(valuesFile, []byte(`{
  "sites": [
    { "name": "cor-it-nl", "hosts": ["cor-it.nl", "www.cor-it.nl"] },
    { "name": "gsid-nl",   "hosts": ["gsid.nl", "www.gsid.nl"] }
  ]
}`), 0644); err != nil {
		t.Fatal(err)
	}

	config, err := LoadWithOptions(LoadOptions{Dir: dir, ValuesFile: valuesFile})
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	if len(config.Ingresses) != 2 {
		t.Fatalf("expected 2 ingresses from the loop, got %d", len(config.Ingresses))
	}

	seen := make(map[string]string) // name -> cert secret
	for _, ing := range config.Ingresses {
		if _, dup := seen[ing.Name]; dup {
			t.Fatalf("duplicate ingress name %q from loop", ing.Name)
		}
		if ing.Namespace != "cms" {
			t.Errorf("ingress %q: expected namespace cms, got %q", ing.Name, ing.Namespace)
		}
		seen[ing.Name] = ing.CertificateSecretName()
	}

	if seen["cms-cor-it-nl"] != "cor-it-nl-tls" {
		t.Errorf("unexpected cert secret for cms-cor-it-nl: %q", seen["cms-cor-it-nl"])
	}
	if seen["cms-gsid-nl"] != "gsid-nl-tls" {
		t.Errorf("unexpected cert secret for cms-gsid-nl: %q", seen["cms-gsid-nl"])
	}
}

func TestStandaloneIngress_RequiresBackend(t *testing.T) {
	cases := map[string]struct {
		body    string
		missing string
	}{
		"no service_name": {body: `ingress "x" { port = 80 }`, missing: "service_name"},
		"no port":         {body: `ingress "x" { service_name = "svc" }`, missing: "port"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := writeRootProject(t, map[string]string{
				"root.kdef":    rootWithIngressDefaults,
				"cms/cms.kdef": tc.body,
			})
			_, err := Load(dir, nil)
			if err == nil {
				t.Fatalf("expected a parse error when %s is omitted", tc.missing)
			}
			if !strings.Contains(err.Error(), tc.missing) {
				t.Errorf("error should name the missing attribute %q, got: %v", tc.missing, err)
			}
		})
	}
}

func TestStandaloneIngress_NameAttributeOverridesLabel(t *testing.T) {
	dir := writeRootProject(t, map[string]string{
		"root.kdef": rootWithIngressDefaults,
		"cms/cms.kdef": `
ingress "placeholder" {
  name         = "cms-gsid-nl"
  service_name = "svc"
  port         = 80
  host         = "gsid.nl"
}
`,
	})
	config, err := Load(dir, nil)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if len(config.Ingresses) != 1 {
		t.Fatalf("expected 1 ingress, got %d", len(config.Ingresses))
	}
	if config.Ingresses[0].Name != "cms-gsid-nl" {
		t.Errorf("explicit name should win over the label, got %q", config.Ingresses[0].Name)
	}
}

func TestValidateIngresses_StandaloneCollidesWithNested_ResourceName(t *testing.T) {
	cfg := &types.KdefConfig{
		Deployments: []types.DeploymentConfig{
			{Name: "web", Namespace: "prod", Ingresses: []types.IngressConfig{
				{Host: "a.example.com"},
			}},
		},
		Ingresses: []types.IngressConfig{
			{Name: "web", Namespace: "prod", ServiceName: "web-svc", Port: 80, Host: "b.example.com"},
		},
	}
	err := validateIngresses(cfg)
	if err == nil {
		t.Fatal("expected a resource name collision")
	}
	if !strings.Contains(err.Error(), "web") {
		t.Errorf("error should reference the colliding name, got: %v", err)
	}
}

func TestValidateIngresses_StandaloneCollidesWithNested_CertSecret(t *testing.T) {
	cfg := &types.KdefConfig{
		Deployments: []types.DeploymentConfig{
			{Name: "web", Namespace: "prod", Ingresses: []types.IngressConfig{
				{Host: "example.com", TLS: true},
			}},
		},
		Ingresses: []types.IngressConfig{
			{Name: "tenant", Namespace: "prod", ServiceName: "web-svc", Port: 80,
				Hosts: []string{"example.com", "www.example.com"}, TLS: true},
		},
	}
	err := validateIngresses(cfg)
	if err == nil {
		t.Fatal("expected a Certificate secret collision")
	}
	if !strings.Contains(err.Error(), "example-com-tls") {
		t.Errorf("error should name the derived secret, got: %v", err)
	}
}

func TestValidateIngresses_TwoStandaloneSameCertSecret(t *testing.T) {
	cfg := &types.KdefConfig{
		Ingresses: []types.IngressConfig{
			{Name: "a", Namespace: "prod", ServiceName: "svc", Port: 80, Host: "example.com", TLS: true},
			{Name: "b", Namespace: "prod", ServiceName: "svc", Port: 80, Host: "example.com", TLS: true},
		},
	}
	if err := validateIngresses(cfg); err == nil {
		t.Fatal("expected a Certificate secret collision between two standalone ingresses")
	}
}

func TestValidateIngresses_StandaloneCrossNamespace_OK(t *testing.T) {
	cfg := &types.KdefConfig{
		Ingresses: []types.IngressConfig{
			{Name: "web", Namespace: "prod", ServiceName: "svc", Port: 80, Host: "example.com", TLS: true},
			{Name: "web", Namespace: "staging", ServiceName: "svc", Port: 80, Host: "example.com", TLS: true},
		},
	}
	if err := validateIngresses(cfg); err != nil {
		t.Fatalf("same name in different namespaces should be fine, got: %v", err)
	}
}
