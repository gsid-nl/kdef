package parser

import (
	"fmt"
	"strings"

	"github.com/gsid-nl/kdef/internal/types"
)

// duplicateCheck records where each resource was defined, so a second
// definition with the same kind, namespace and name is reported with both
// origins instead of one quietly replacing the other on apply.
type duplicateCheck struct {
	origins map[string]string
	errs    []string
}

func newDuplicateCheck() *duplicateCheck {
	return &duplicateCheck{origins: make(map[string]string)}
}

// add records every resource in config as coming from origin.
func (d *duplicateCheck) add(config *types.KdefConfig, origin string) {
	for _, r := range resourcesOf(config) {
		// A duplicated workload also duplicates its Service; report it once.
		if d.record(r.id, origin) && r.service != "" {
			d.record(r.service, origin)
		}
	}
}

// record returns false when id was already taken.
func (d *duplicateCheck) record(id, origin string) bool {
	prev, dup := d.origins[id]
	if !dup {
		d.origins[id] = origin
		return true
	}
	if prev == origin {
		d.errs = append(d.errs, fmt.Sprintf("%s is defined twice in %s", id, origin))
	} else {
		d.errs = append(d.errs, fmt.Sprintf("%s is defined in both %s and %s", id, prev, origin))
	}
	return false
}

func (d *duplicateCheck) err() error {
	if len(d.errs) == 0 {
		return nil
	}
	return fmt.Errorf("duplicate resources:\n  %s", strings.Join(d.errs, "\n  "))
}

// asConfig wraps one file's blocks so they can be checked like a full config.
func (r FileResult) asConfig() *types.KdefConfig {
	return &types.KdefConfig{
		Deployments:            r.Deployments,
		DaemonSets:             r.DaemonSets,
		StatefulSets:           r.StatefulSets,
		CronJobs:               r.CronJobs,
		ConfigMaps:             r.ConfigMaps,
		Ingresses:              r.Ingresses,
		Secrets:                r.Secrets,
		SealedSecrets:          r.SealedSecrets,
		PersistentVolumeClaims: r.PersistentVolumeClaims,
		ClusterRoles:           r.ClusterRoles,
		ClusterRoleBindings:    r.ClusterRoleBindings,
		CRDs:                   r.CRDs,
		CSIDrivers:             r.CSIDrivers,
	}
}

type resourceID struct {
	id      string
	service string // the Service a workload generates, if any
}

func resourceIdentity(kind, namespace, name string) string {
	if namespace == "" {
		return fmt.Sprintf("%s %q", kind, name)
	}
	return fmt.Sprintf("%s %q in namespace %q", kind, name, namespace)
}

// resourcesOf lists the resources config declares, plus the Service each
// workload generates: a deployment and a statefulset of the same name would
// otherwise both write Service "<name>".
func resourcesOf(config *types.KdefConfig) []resourceID {
	var out []resourceID

	workload := func(kind, namespace, name string, svc *types.ServiceConfig) {
		r := resourceID{id: resourceIdentity(kind, namespace, name)}
		if svc != nil {
			svcName := name
			if svc.Name != "" {
				svcName = svc.Name
			}
			r.service = resourceIdentity("service", namespace, svcName)
		}
		out = append(out, r)
	}
	plain := func(kind, namespace, name string) {
		out = append(out, resourceID{id: resourceIdentity(kind, namespace, name)})
	}

	for _, r := range config.Deployments {
		workload("deployment", r.Namespace, r.Name, r.Service)
	}
	for _, r := range config.DaemonSets {
		workload("daemonset", r.Namespace, r.Name, r.Service)
	}
	for _, r := range config.StatefulSets {
		workload("statefulset", r.Namespace, r.Name, r.Service)
	}
	for _, r := range config.CronJobs {
		plain("cronjob", r.Namespace, r.Name)
	}
	for _, r := range config.ConfigMaps {
		plain("configmap", r.Namespace, r.Name)
	}
	for _, r := range config.Secrets {
		plain("secret", r.Namespace, r.Name)
	}
	for _, r := range config.SealedSecrets {
		plain("sealedsecret", r.Namespace, r.Name)
	}
	for _, r := range config.PersistentVolumeClaims {
		plain("persistentvolumeclaim", r.Namespace, r.Name)
	}
	for _, r := range config.Ingresses {
		plain("ingress", r.Namespace, r.Name)
	}

	// Cluster-scoped: the name alone identifies them.
	for _, r := range config.ClusterRoles {
		plain("clusterrole", "", r.Name)
	}
	for _, r := range config.ClusterRoleBindings {
		plain("clusterrolebinding", "", r.Name)
	}
	for _, r := range config.CRDs {
		plain("crd", "", r.Name)
	}
	for _, r := range config.CSIDrivers {
		plain("csidriver", "", r.Name)
	}
	return out
}
