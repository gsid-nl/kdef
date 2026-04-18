package generator

import (
	"github.com/gsid-nl/kdef/internal/types"
)

// Manifest holds a rendered K8s object ready for YAML serialization.
type Manifest struct {
	Object interface{}
	Raw    string // optional raw YAML to deep-merge into the output
}

// GenerateCronJobManifest produces manifests for a cronjob.
func GenerateCronJobManifest(cj types.CronJobConfig) []Manifest {
	return []Manifest{{Object: GenerateCronJob(cj)}}
}

// Generate produces all K8s manifests for a full kdef config.
func Generate(config *types.KdefConfig) map[string][]Manifest {
	result := make(map[string][]Manifest)

	// Namespaces
	for _, ns := range config.Namespaces {
		result["namespace-"+ns] = []Manifest{{Object: GenerateNamespace(ns)}}
	}

	// ServiceAccounts — generate in each namespace where they're referenced
	saNamespaces := collectServiceAccountNamespaces(config)
	for _, sa := range config.ServiceAccounts {
		for _, ns := range saNamespaces[sa.Name] {
			key := "sa-" + sa.Name + "-" + ns
			result[key] = []Manifest{{Object: GenerateServiceAccount(sa, ns)}}
		}
	}

	for _, dep := range config.Deployments {
		result[dep.Name] = GenerateDeploymentV2(dep)
	}
	for _, ds := range config.DaemonSets {
		result[ds.Name] = GenerateDaemonSet(ds)
	}
	for _, sts := range config.StatefulSets {
		result[sts.Name] = GenerateStatefulSet(sts)
	}
	for _, cj := range config.CronJobs {
		result[cj.Name] = GenerateCronJobManifest(cj)
	}
	for _, cm := range config.ConfigMaps {
		result["configmap-"+cm.Name] = []Manifest{{Object: GenerateConfigMap(cm)}}
	}
	for _, s := range config.Secrets {
		result["secret-"+s.Name] = []Manifest{{Object: GenerateSecret(s)}}
	}
	for _, ss := range config.SealedSecrets {
		result["sealedsecret-"+ss.Name] = []Manifest{{Object: GenerateSealedSecret(ss)}}
	}
	for _, pvc := range config.PersistentVolumeClaims {
		result["pvc-"+pvc.Name] = []Manifest{{Object: GeneratePersistentVolumeClaim(pvc)}}
	}
	return result
}

// collectServiceAccountNamespaces finds which namespaces each service account is used in.
func collectServiceAccountNamespaces(config *types.KdefConfig) map[string][]string {
	seen := make(map[string]map[string]bool) // sa name -> set of namespaces
	for _, dep := range config.Deployments {
		if dep.ServiceAccountName != "" {
			if seen[dep.ServiceAccountName] == nil {
				seen[dep.ServiceAccountName] = make(map[string]bool)
			}
			seen[dep.ServiceAccountName][dep.Namespace] = true
		}
	}
	for _, ds := range config.DaemonSets {
		if ds.ServiceAccountName != "" {
			if seen[ds.ServiceAccountName] == nil {
				seen[ds.ServiceAccountName] = make(map[string]bool)
			}
			seen[ds.ServiceAccountName][ds.Namespace] = true
		}
	}
	for _, sts := range config.StatefulSets {
		if sts.ServiceAccountName != "" {
			if seen[sts.ServiceAccountName] == nil {
				seen[sts.ServiceAccountName] = make(map[string]bool)
			}
			seen[sts.ServiceAccountName][sts.Namespace] = true
		}
	}
	for _, cj := range config.CronJobs {
		if cj.ServiceAccountName != "" {
			if seen[cj.ServiceAccountName] == nil {
				seen[cj.ServiceAccountName] = make(map[string]bool)
			}
			seen[cj.ServiceAccountName][cj.Namespace] = true
		}
	}

	result := make(map[string][]string)
	for name, nsSet := range seen {
		for ns := range nsSet {
			result[name] = append(result[name], ns)
		}
	}
	return result
}
