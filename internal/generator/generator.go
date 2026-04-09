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
	for _, dep := range config.Deployments {
		result[dep.Name] = GenerateDeploymentV2(dep)
	}
	for _, cj := range config.CronJobs {
		result[cj.Name] = GenerateCronJobManifest(cj)
	}
	for _, cm := range config.ConfigMaps {
		result["configmap-"+cm.Name] = []Manifest{{Object: GenerateConfigMap(cm)}}
	}
	for _, ss := range config.SealedSecrets {
		result["sealedsecret-"+ss.Name] = []Manifest{{Object: GenerateSealedSecret(ss)}}
	}
	for _, pvc := range config.PersistentVolumeClaims {
		result["pvc-"+pvc.Name] = []Manifest{{Object: GeneratePersistentVolumeClaim(pvc)}}
	}
	return result
}
