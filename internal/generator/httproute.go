package generator

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/gsid-nl/kdef/internal/types"
)

// GenerateHTTPRoute creates a Gateway API HTTPRoute for an app.
// Returns nil if no ingress block is defined.
func GenerateHTTPRoute(app types.AppConfig) *gatewayv1.HTTPRoute {
	if app.Ingress == nil {
		return nil
	}

	hosts := app.Ingress.AllHosts()
	if len(hosts) == 0 {
		return nil
	}

	// Backend service name — defaults to app name
	svcName := app.Name
	if app.Ingress.ServiceName != "" {
		svcName = app.Ingress.ServiceName
	}

	// Backend port: explicit ingress port > first app port > 80
	var backendPort int32 = 80
	if app.Ingress.Port > 0 {
		backendPort = app.Ingress.Port
	} else if len(app.Ports) > 0 {
		backendPort = app.Ports[0].Number
	}

	portNum := gatewayv1.PortNumber(backendPort)
	pathMatchType := gatewayv1.PathMatchPathPrefix
	pathValue := "/"

	rules := []gatewayv1.HTTPRouteRule{
		{
			Matches: []gatewayv1.HTTPRouteMatch{
				{
					Path: &gatewayv1.HTTPPathMatch{
						Type:  &pathMatchType,
						Value: &pathValue,
					},
				},
			},
			BackendRefs: []gatewayv1.HTTPBackendRef{
				{
					BackendRef: gatewayv1.BackendRef{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name: gatewayv1.ObjectName(svcName),
							Port: &portNum,
						},
					},
				},
			},
		},
	}

	// parentRef — the Gateway this HTTPRoute attaches to
	parentRef := gatewayv1.ParentReference{
		Name: gatewayv1.ObjectName(app.IngressGateway),
	}
	if app.IngressGatewayNS != "" {
		ns := gatewayv1.Namespace(app.IngressGatewayNS)
		parentRef.Namespace = &ns
	}

	// hostnames
	var hostnames []gatewayv1.Hostname
	for _, h := range hosts {
		hostnames = append(hostnames, gatewayv1.Hostname(h))
	}

	route := &gatewayv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "gateway.networking.k8s.io/v1",
			Kind:       "HTTPRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressName(app),
			Namespace: app.Namespace,
			Labels: kdefLabels(map[string]string{
				"app.kubernetes.io/name": app.Name,
			}),
			Annotations: app.Ingress.Annotations,
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{parentRef},
			},
			Hostnames: hostnames,
			Rules:     rules,
		},
	}

	return route
}
