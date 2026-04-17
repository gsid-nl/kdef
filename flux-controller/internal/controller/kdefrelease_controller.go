package controller

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	kdefv1alpha1 "github.com/gsid-nl/kdef/flux-controller/api/v1alpha1"
	"github.com/gsid-nl/kdef/internal/generator"
	"github.com/gsid-nl/kdef/internal/parser"

	fluxmeta "github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
)

const (
	fieldManager          = "kdef-controller"
	reconcileRequestAnnotation = "reconcile.fluxcd.io/requestedAt"
)

// KdefReleaseReconciler reconciles a KdefRelease object.
type KdefReleaseReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	HTTPClient *http.Client
}

// +kubebuilder:rbac:groups=kdef.gsid.nl,resources=kdefreleases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kdef.gsid.nl,resources=kdefreleases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=gitrepositories;ocirepositories;buckets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps;secrets;services;serviceaccounts;namespaces;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=bitnami.com,resources=sealedsecrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates,verbs=get;list;watch;create;update;patch;delete

func (r *KdefReleaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the KdefRelease
	release := &kdefv1alpha1.KdefRelease{}
	if err := r.Get(ctx, req.NamespacedName, release); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Check if suspended
	if release.Spec.Suspend {
		logger.Info("reconciliation suspended")
		return ctrl.Result{}, nil
	}

	// Set reconciling condition (use patch to avoid resource version conflicts)
	patch := client.MergeFrom(release.DeepCopy())
	meta.SetStatusCondition(&release.Status.Conditions, metav1.Condition{
		Type:               "Reconciling",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciling",
		Message:            "Reconciliation in progress",
		ObservedGeneration: release.Generation,
	})
	if err := r.Status().Patch(ctx, release, patch); err != nil {
		return ctrl.Result{}, err
	}

	// Resolve the source artifact
	artifact, err := r.getSourceArtifact(ctx, release)
	if err != nil {
		return r.failWithError(ctx, release, "SourceNotReady", err)
	}

	release.Status.LastAttemptedRevision = artifact.Revision

	// Track manual reconcile requests via annotation
	if requestedAt, ok := release.GetAnnotations()[reconcileRequestAnnotation]; ok {
		if requestedAt != release.Status.LastHandledReconcileAt {
			release.Status.LastHandledReconcileAt = requestedAt
			logger.Info("manual reconcile requested", "requestedAt", requestedAt)
		}
	}

	// Download and extract the artifact
	tmpDir, err := r.fetchArtifact(ctx, artifact)
	if err != nil {
		return r.failWithError(ctx, release, "ArtifactFetchFailed", err)
	}
	defer os.RemoveAll(tmpDir)

	// Determine the kdef project directory
	projectDir := tmpDir
	if release.Spec.Path != "" {
		projectDir = filepath.Join(tmpDir, release.Spec.Path)
	}

	// Load values from ConfigMap/Secret if specified
	valuesFile := ""
	if release.Spec.ValuesFrom != nil {
		valuesFile, err = r.resolveValuesFrom(ctx, release)
		if err != nil {
			return r.failWithError(ctx, release, "ValuesResolveFailed", err)
		}
		if valuesFile != "" {
			defer os.Remove(valuesFile)
		}
	}

	// Render kdef
	objects, err := r.renderKdef(projectDir, release.Spec, valuesFile)
	if err != nil {
		return r.failWithError(ctx, release, "RenderFailed", err)
	}

	logger.Info("rendered kdef manifests", "count", len(objects))

	// Override target namespace if specified
	if release.Spec.TargetNamespace != "" {
		for i := range objects {
			objects[i].SetNamespace(release.Spec.TargetNamespace)
		}
	}

	// Apply via server-side apply
	newInventory, err := r.applyObjects(ctx, objects)
	if err != nil {
		logger.Error(err, "failed to apply objects")
		return r.failWithError(ctx, release, "ApplyFailed", err)
	}

	// Prune removed resources
	if release.Spec.Prune && release.Status.Inventory != nil {
		pruned, err := r.pruneObjects(ctx, release.Status.Inventory, newInventory)
		if err != nil {
			logger.Error(err, "pruning failed")
		} else if pruned > 0 {
			logger.Info("pruned stale resources", "count", pruned)
		}
	}

	// Update status
	successPatch := client.MergeFrom(release.DeepCopy())
	release.Status.LastAppliedRevision = artifact.Revision
	release.Status.Inventory = newInventory
	release.Status.ObservedGeneration = release.Generation
	meta.SetStatusCondition(&release.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Succeeded",
		Message:            fmt.Sprintf("Applied revision: %s (%d resources)", artifact.Revision, len(objects)),
		ObservedGeneration: release.Generation,
	})
	meta.RemoveStatusCondition(&release.Status.Conditions, "Reconciling")
	if err := r.Status().Patch(ctx, release, successPatch); err != nil {
		logger.Error(err, "failed to patch success status")
		return ctrl.Result{}, err
	}

	logger.Info("reconciliation succeeded", "revision", artifact.Revision, "resources", len(objects))
	return ctrl.Result{RequeueAfter: release.Spec.Interval.Duration}, nil
}

// getSourceArtifact resolves the Flux source and returns the latest artifact.
func (r *KdefReleaseReconciler) getSourceArtifact(ctx context.Context, release *kdefv1alpha1.KdefRelease) (*fluxmeta.Artifact, error) {
	ref := release.Spec.SourceRef
	ns := ref.Namespace
	if ns == "" {
		ns = release.Namespace
	}

	namespacedName := types.NamespacedName{Name: ref.Name, Namespace: ns}

	switch ref.Kind {
	case "GitRepository":
		repo := &sourcev1.GitRepository{}
		if err := r.Get(ctx, namespacedName, repo); err != nil {
			return nil, fmt.Errorf("get GitRepository %s: %w", namespacedName, err)
		}
		if repo.Status.Artifact == nil {
			return nil, fmt.Errorf("GitRepository %s has no artifact", namespacedName)
		}
		return repo.Status.Artifact, nil
	case "OCIRepository":
		repo := &sourcev1.OCIRepository{}
		if err := r.Get(ctx, namespacedName, repo); err != nil {
			return nil, fmt.Errorf("get OCIRepository %s: %w", namespacedName, err)
		}
		if repo.Status.Artifact == nil {
			return nil, fmt.Errorf("OCIRepository %s has no artifact", namespacedName)
		}
		return repo.Status.Artifact, nil
	default:
		return nil, fmt.Errorf("unsupported source kind: %s", ref.Kind)
	}
}

// fetchArtifact downloads and extracts a Flux artifact tarball.
func (r *KdefReleaseReconciler) fetchArtifact(ctx context.Context, artifact *fluxmeta.Artifact) (string, error) {
	httpClient := r.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.URL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download artifact: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download artifact: HTTP %d", resp.StatusCode)
	}

	// Read body and verify checksum
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read artifact: %w", err)
	}

	if artifact.Digest != "" {
		// Digest format: sha256:hex
		parts := strings.SplitN(artifact.Digest, ":", 2)
		if len(parts) == 2 && parts[0] == "sha256" {
			h := sha256.Sum256(body)
			got := hex.EncodeToString(h[:])
			if got != parts[1] {
				return "", fmt.Errorf("checksum mismatch: expected %s, got %s", parts[1], got)
			}
		}
	}

	// Extract to temp directory
	tmpDir, err := os.MkdirTemp("", "kdef-artifact-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	if err := extractTarGz(body, tmpDir); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("extract artifact: %w", err)
	}

	return tmpDir, nil
}

// extractTarGz extracts a tar.gz archive to a directory.
func extractTarGz(data []byte, destDir string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destDir, header.Name)

		// Prevent path traversal
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)) {
			return fmt.Errorf("path traversal detected: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, io.LimitReader(tr, 50<<20)); err != nil { // 50MB limit per file
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}

// renderKdef runs the kdef render pipeline on a project directory.
func (r *KdefReleaseReconciler) renderKdef(projectDir string, spec kdefv1alpha1.KdefReleaseSpec, valuesFile string) ([]*unstructured.Unstructured, error) {
	opts := parser.LoadOptions{
		Dir:        projectDir,
		Overrides:  spec.Set,
		Env:        spec.Env,
		ValuesFile: valuesFile,
	}

	config, err := parser.LoadWithOptions(opts)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	allManifests := generator.Generate(config)
	yamlBytes, err := generator.RenderAllYAML(allManifests)
	if err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}

	return parseYAMLObjects(yamlBytes)
}

// parseYAMLObjects splits multi-document YAML into unstructured objects.
func parseYAMLObjects(yamlBytes []byte) ([]*unstructured.Unstructured, error) {
	var objects []*unstructured.Unstructured

	decoder := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), 4096)
	for {
		obj := &unstructured.Unstructured{}
		err := decoder.Decode(obj)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode YAML: %w", err)
		}
		if obj.GetKind() == "" {
			continue // skip empty documents
		}
		objects = append(objects, obj)
	}

	return objects, nil
}

// applyOrder returns a sort priority for a Kubernetes resource kind.
// Lower values are applied first (e.g. Namespace before Deployment).
func applyOrder(kind string) int {
	switch kind {
	case "Namespace":
		return 0
	case "ConfigMap", "Secret", "SealedSecret":
		return 1
	case "PersistentVolumeClaim":
		return 2
	case "ServiceAccount":
		return 3
	case "Service":
		return 4
	case "Deployment", "CronJob":
		return 5
	case "Ingress", "Certificate":
		return 6
	case "HorizontalPodAutoscaler":
		return 7
	default:
		return 8
	}
}

// applyObjects applies all objects via server-side apply and returns the inventory.
func (r *KdefReleaseReconciler) applyObjects(ctx context.Context, objects []*unstructured.Unstructured) (*kdefv1alpha1.ResourceInventory, error) {
	inventory := &kdefv1alpha1.ResourceInventory{}

	// Sort so namespaces and dependencies are created before workloads
	sort.SliceStable(objects, func(i, j int) bool {
		return applyOrder(objects[i].GetKind()) < applyOrder(objects[j].GetKind())
	})

	for _, obj := range objects {
		// Server-side apply
		data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, obj)
		if err != nil {
			return inventory, fmt.Errorf("encode %s/%s: %w", obj.GetKind(), obj.GetName(), err)
		}

		patch := client.RawPatch(types.ApplyPatchType, data)
		applied := obj.DeepCopy()
		err = r.Patch(ctx, applied, patch, client.FieldOwner(fieldManager), client.ForceOwnership)
		if err != nil {
			return inventory, fmt.Errorf("apply %s/%s (%s): %w", obj.GetKind(), obj.GetName(), obj.GetNamespace(), err)
		}

		// Add to inventory
		inventory.Entries = append(inventory.Entries, kdefv1alpha1.ResourceRef{
			ID:      resourceID(obj),
			Version: obj.GetAPIVersion(),
		})
	}

	return inventory, nil
}

// pruneObjects deletes resources that are in the old inventory but not in the new one.
func (r *KdefReleaseReconciler) pruneObjects(ctx context.Context, oldInventory, newInventory *kdefv1alpha1.ResourceInventory) (int, error) {
	newIDs := make(map[string]bool)
	for _, ref := range newInventory.Entries {
		newIDs[ref.ID] = true
	}

	pruned := 0
	for _, ref := range oldInventory.Entries {
		if newIDs[ref.ID] {
			continue
		}

		// Parse the resource ID and delete
		obj, err := objectFromResourceRef(ref)
		if err != nil {
			continue
		}

		if err := r.Delete(ctx, obj); err != nil {
			if !errors.IsNotFound(err) {
				return pruned, fmt.Errorf("delete %s: %w", ref.ID, err)
			}
		}
		pruned++
	}
	return pruned, nil
}

// resolveValuesFrom reads values from a ConfigMap or Secret.
func (r *KdefReleaseReconciler) resolveValuesFrom(ctx context.Context, release *kdefv1alpha1.KdefRelease) (string, error) {
	vf := release.Spec.ValuesFrom
	key := vf.Key
	if key == "" {
		key = "values.json"
	}

	var data string
	switch vf.Kind {
	case "ConfigMap":
		cm := &corev1.ConfigMap{}
		if err := r.Get(ctx, types.NamespacedName{Name: vf.Name, Namespace: release.Namespace}, cm); err != nil {
			return "", fmt.Errorf("get ConfigMap %s: %w", vf.Name, err)
		}
		var ok bool
		data, ok = cm.Data[key]
		if !ok {
			return "", fmt.Errorf("ConfigMap %s has no key %q", vf.Name, key)
		}
	case "Secret":
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: vf.Name, Namespace: release.Namespace}, secret); err != nil {
			return "", fmt.Errorf("get Secret %s: %w", vf.Name, err)
		}
		b, ok := secret.Data[key]
		if !ok {
			return "", fmt.Errorf("Secret %s has no key %q", vf.Name, key)
		}
		data = string(b)
	default:
		return "", fmt.Errorf("unsupported ValuesFrom kind: %s", vf.Kind)
	}

	// Write to temp file
	f, err := os.CreateTemp("", "kdef-values-*.json")
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	f.Close()
	return f.Name(), nil
}

// failWithError sets the Ready condition to False and returns a requeue.
func (r *KdefReleaseReconciler) failWithError(ctx context.Context, release *kdefv1alpha1.KdefRelease, reason string, err error) (ctrl.Result, error) {
	patch := client.MergeFrom(release.DeepCopy())
	meta.SetStatusCondition(&release.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            err.Error(),
		ObservedGeneration: release.Generation,
	})
	meta.RemoveStatusCondition(&release.Status.Conditions, "Reconciling")
	if statusErr := r.Status().Patch(ctx, release, patch); statusErr != nil {
		log.FromContext(ctx).Error(statusErr, "failed to patch failure status")
		return ctrl.Result{}, statusErr
	}
	// Retry after interval
	return ctrl.Result{RequeueAfter: release.Spec.Interval.Duration}, nil
}

// resourceID builds a unique ID for an unstructured object: namespace_name_group_kind.
func resourceID(obj *unstructured.Unstructured) string {
	gvk := obj.GroupVersionKind()
	return fmt.Sprintf("%s_%s_%s_%s", obj.GetNamespace(), obj.GetName(), gvk.Group, gvk.Kind)
}

// objectFromResourceRef reconstructs a minimal unstructured object for deletion.
func objectFromResourceRef(ref kdefv1alpha1.ResourceRef) (*unstructured.Unstructured, error) {
	// ID format: namespace_name_group_kind
	parts := strings.SplitN(ref.ID, "_", 4)
	if len(parts) != 4 {
		return nil, fmt.Errorf("invalid resource ID: %s", ref.ID)
	}

	obj := &unstructured.Unstructured{}
	obj.SetNamespace(parts[0])
	obj.SetName(parts[1])
	obj.SetAPIVersion(ref.Version)
	obj.SetKind(parts[3])

	return obj, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KdefReleaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kdefv1alpha1.KdefRelease{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{},
			predicate.AnnotationChangedPredicate{},
		))).
		Complete(r)
}

// Ensure the http client has a reasonable timeout
func init() {
	http.DefaultClient.Timeout = 30 * time.Second
}
