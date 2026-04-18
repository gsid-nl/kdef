package importer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// Annotations/labels to strip during import — these are K8s internal bookkeeping.
var noiseAnnotations = []string{
	"kubectl.kubernetes.io/last-applied-configuration",
	"deployment.kubernetes.io/revision",
	"field.cattle.io/",
	"meta.helm.sh/",
	"argocd.argoproj.io/",
}

func cleanMetadata(obj *metav1.ObjectMeta) {
	for k := range obj.Annotations {
		for _, noise := range noiseAnnotations {
			if k == noise || strings.HasPrefix(k, noise) {
				delete(obj.Annotations, k)
			}
		}
	}
	if len(obj.Annotations) == 0 {
		obj.Annotations = nil
	}
	// Remove managed-fields, resourceVersion, uid, etc. (already not serialized by typed objects,
	// but just in case)
	obj.ManagedFields = nil
	obj.ResourceVersion = ""
	obj.UID = ""
	obj.Generation = 0
	obj.CreationTimestamp = metav1.Time{}
}

// ClusterResources holds all resources read from a cluster or file.
type ClusterResources struct {
	Deployments            []appsv1.Deployment
	DaemonSets             []appsv1.DaemonSet
	StatefulSets           []appsv1.StatefulSet
	Services               []corev1.Service
	Ingresses              []networkingv1.Ingress
	CronJobs               []batchv1.CronJob
	ConfigMaps             []corev1.ConfigMap
	PersistentVolumeClaims []corev1.PersistentVolumeClaim
}

// ReadFromCluster reads resources from the current kubectl context.
func ReadFromCluster(namespace string) (*ClusterResources, error) {
	resources := &ClusterResources{}

	// Deployments
	deployments, err := kubectlGet("deployments", namespace)
	if err != nil {
		return nil, fmt.Errorf("get deployments: %w", err)
	}
	for _, raw := range deployments {
		var dep appsv1.Deployment
		if err := json.Unmarshal(raw, &dep); err != nil {
			return nil, fmt.Errorf("unmarshal deployment: %w", err)
		}
		resources.Deployments = append(resources.Deployments, dep)
	}

	// DaemonSets
	daemonsets, err := kubectlGet("daemonsets", namespace)
	if err != nil {
		return nil, fmt.Errorf("get daemonsets: %w", err)
	}
	for _, raw := range daemonsets {
		var ds appsv1.DaemonSet
		if err := json.Unmarshal(raw, &ds); err != nil {
			return nil, fmt.Errorf("unmarshal daemonset: %w", err)
		}
		resources.DaemonSets = append(resources.DaemonSets, ds)
	}

	// StatefulSets
	statefulsets, err := kubectlGet("statefulsets", namespace)
	if err != nil {
		return nil, fmt.Errorf("get statefulsets: %w", err)
	}
	for _, raw := range statefulsets {
		var sts appsv1.StatefulSet
		if err := json.Unmarshal(raw, &sts); err != nil {
			return nil, fmt.Errorf("unmarshal statefulset: %w", err)
		}
		resources.StatefulSets = append(resources.StatefulSets, sts)
	}

	// Services
	services, err := kubectlGet("services", namespace)
	if err != nil {
		return nil, fmt.Errorf("get services: %w", err)
	}
	for _, raw := range services {
		var svc corev1.Service
		if err := json.Unmarshal(raw, &svc); err != nil {
			return nil, fmt.Errorf("unmarshal service: %w", err)
		}
		resources.Services = append(resources.Services, svc)
	}

	// Ingresses
	ingresses, err := kubectlGet("ingresses", namespace)
	if err != nil {
		return nil, fmt.Errorf("get ingresses: %w", err)
	}
	for _, raw := range ingresses {
		var ing networkingv1.Ingress
		if err := json.Unmarshal(raw, &ing); err != nil {
			return nil, fmt.Errorf("unmarshal ingress: %w", err)
		}
		resources.Ingresses = append(resources.Ingresses, ing)
	}

	// CronJobs
	cronJobs, err := kubectlGet("cronjobs", namespace)
	if err != nil {
		return nil, fmt.Errorf("get cronjobs: %w", err)
	}
	for _, raw := range cronJobs {
		var cj batchv1.CronJob
		if err := json.Unmarshal(raw, &cj); err != nil {
			return nil, fmt.Errorf("unmarshal cronjob: %w", err)
		}
		resources.CronJobs = append(resources.CronJobs, cj)
	}

	// ConfigMaps
	configMaps, err := kubectlGet("configmaps", namespace)
	if err != nil {
		return nil, fmt.Errorf("get configmaps: %w", err)
	}
	for _, raw := range configMaps {
		var cm corev1.ConfigMap
		if err := json.Unmarshal(raw, &cm); err != nil {
			return nil, fmt.Errorf("unmarshal configmap: %w", err)
		}
		// Skip kube-system configmaps
		if cm.Name == "kube-root-ca.crt" {
			continue
		}
		resources.ConfigMaps = append(resources.ConfigMaps, cm)
	}

	// PersistentVolumeClaims
	pvcs, err := kubectlGet("persistentvolumeclaims", namespace)
	if err != nil {
		return nil, fmt.Errorf("get persistentvolumeclaims: %w", err)
	}
	for _, raw := range pvcs {
		var pvc corev1.PersistentVolumeClaim
		if err := json.Unmarshal(raw, &pvc); err != nil {
			return nil, fmt.Errorf("unmarshal persistentvolumeclaim: %w", err)
		}
		resources.PersistentVolumeClaims = append(resources.PersistentVolumeClaims, pvc)
	}

	cleanAllResources(resources)
	return resources, nil
}

// ReadFromFile reads resources from a YAML file (multi-document).
func ReadFromFile(filename string) (*ClusterResources, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	resources := &ClusterResources{}

	// Split on YAML document separator
	docs := splitYAMLDocs(data)
	for _, doc := range docs {
		if len(doc) == 0 {
			continue
		}

		// Peek at kind
		var meta struct {
			Kind string `json:"kind"`
		}
		jsonData, err := yaml.YAMLToJSON(doc)
		if err != nil {
			continue // skip unparseable docs
		}
		if err := json.Unmarshal(jsonData, &meta); err != nil {
			continue
		}

		switch meta.Kind {
		case "Deployment":
			var dep appsv1.Deployment
			if err := json.Unmarshal(jsonData, &dep); err == nil {
				resources.Deployments = append(resources.Deployments, dep)
			}
		case "DaemonSet":
			var ds appsv1.DaemonSet
			if err := json.Unmarshal(jsonData, &ds); err == nil {
				resources.DaemonSets = append(resources.DaemonSets, ds)
			}
		case "StatefulSet":
			var sts appsv1.StatefulSet
			if err := json.Unmarshal(jsonData, &sts); err == nil {
				resources.StatefulSets = append(resources.StatefulSets, sts)
			}
		case "Service":
			var svc corev1.Service
			if err := json.Unmarshal(jsonData, &svc); err == nil {
				resources.Services = append(resources.Services, svc)
			}
		case "Ingress":
			var ing networkingv1.Ingress
			if err := json.Unmarshal(jsonData, &ing); err == nil {
				resources.Ingresses = append(resources.Ingresses, ing)
			}
		case "CronJob":
			var cj batchv1.CronJob
			if err := json.Unmarshal(jsonData, &cj); err == nil {
				resources.CronJobs = append(resources.CronJobs, cj)
			}
		case "ConfigMap":
			var cm corev1.ConfigMap
			if err := json.Unmarshal(jsonData, &cm); err == nil {
				resources.ConfigMaps = append(resources.ConfigMaps, cm)
			}
		case "PersistentVolumeClaim":
			var pvc corev1.PersistentVolumeClaim
			if err := json.Unmarshal(jsonData, &pvc); err == nil {
				resources.PersistentVolumeClaims = append(resources.PersistentVolumeClaims, pvc)
			}
		}
	}

	cleanAllResources(resources)
	return resources, nil
}

func kubectlGet(resource, namespace string) ([]json.RawMessage, error) {
	cmd := exec.Command("kubectl", "get", resource, "-n", namespace, "-o", "json")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("%s: %s", resource, string(exitErr.Stderr))
		}
		return nil, err
	}

	var list struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, fmt.Errorf("parse kubectl output: %w", err)
	}

	return list.Items, nil
}

func splitYAMLDocs(data []byte) [][]byte {
	var docs [][]byte
	var current []byte

	for _, line := range splitLines(data) {
		if string(line) == "---" {
			if len(current) > 0 {
				docs = append(docs, current)
				current = nil
			}
			continue
		}
		current = append(current, line...)
		current = append(current, '\n')
	}
	if len(current) > 0 {
		docs = append(docs, current)
	}

	return docs
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

func cleanAllResources(r *ClusterResources) {
	for i := range r.Deployments {
		cleanMetadata(&r.Deployments[i].ObjectMeta)
		// Also clean pod template annotations
		cleanMetadata(&r.Deployments[i].Spec.Template.ObjectMeta)
	}
	for i := range r.DaemonSets {
		cleanMetadata(&r.DaemonSets[i].ObjectMeta)
		cleanMetadata(&r.DaemonSets[i].Spec.Template.ObjectMeta)
	}
	for i := range r.StatefulSets {
		cleanMetadata(&r.StatefulSets[i].ObjectMeta)
		cleanMetadata(&r.StatefulSets[i].Spec.Template.ObjectMeta)
	}
	for i := range r.Services {
		cleanMetadata(&r.Services[i].ObjectMeta)
	}
	for i := range r.Ingresses {
		cleanMetadata(&r.Ingresses[i].ObjectMeta)
	}
	for i := range r.CronJobs {
		cleanMetadata(&r.CronJobs[i].ObjectMeta)
		cleanMetadata(&r.CronJobs[i].Spec.JobTemplate.ObjectMeta)
	}
	for i := range r.ConfigMaps {
		cleanMetadata(&r.ConfigMaps[i].ObjectMeta)
	}
	for i := range r.PersistentVolumeClaims {
		cleanMetadata(&r.PersistentVolumeClaims[i].ObjectMeta)
	}
}
