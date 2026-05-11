package importer

import (
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

// AppGroup groups related K8s resources that form a single kdef app/worker.
type AppGroup struct {
	Name        string
	Deployment  *appsv1.Deployment
	DaemonSet   *appsv1.DaemonSet
	StatefulSet *appsv1.StatefulSet
	Service     *corev1.Service
	Ingresses   []*networkingv1.Ingress
}

// ImportResult holds all generated kdef blocks.
type ImportResult struct {
	Deployments            []string // rendered kdef deployment blocks
	DaemonSets             []string // rendered kdef daemonset blocks
	StatefulSets           []string // rendered kdef statefulset blocks
	CronJobs               []string // rendered kdef cronjob blocks
	ConfigMaps             []string // rendered kdef configmap blocks
	PersistentVolumeClaims []string // rendered kdef persistentvolumeclaim blocks
	ClusterRoles           []string // rendered kdef clusterrole blocks
	ClusterRoleBindings    []string // rendered kdef clusterrolebinding blocks
}

// MapToKdef converts cluster resources to kdef syntax strings.
func MapToKdef(resources *ClusterResources) ImportResult {
	var result ImportResult

	// Group deployments with their services and ingresses
	depGroups, dsGroups, stsGroups := groupWorkloads(resources)

	for _, group := range depGroups {
		result.Deployments = append(result.Deployments, renderDeploymentBlock(group))
	}
	for _, group := range dsGroups {
		result.DaemonSets = append(result.DaemonSets, renderDaemonSetBlock(group))
	}
	for _, group := range stsGroups {
		result.StatefulSets = append(result.StatefulSets, renderStatefulSetBlock(group))
	}

	// CronJobs are standalone
	for _, cj := range resources.CronJobs {
		result.CronJobs = append(result.CronJobs, renderCronJobBlock(cj))
	}

	// ConfigMaps
	for _, cm := range resources.ConfigMaps {
		result.ConfigMaps = append(result.ConfigMaps, renderConfigMapBlock(cm))
	}

	// PersistentVolumeClaims
	for _, pvc := range resources.PersistentVolumeClaims {
		result.PersistentVolumeClaims = append(result.PersistentVolumeClaims, renderPersistentVolumeClaimBlock(pvc))
	}

	// ClusterRoles
	for _, cr := range resources.ClusterRoles {
		result.ClusterRoles = append(result.ClusterRoles, renderClusterRoleBlock(cr))
	}

	// ClusterRoleBindings
	for _, crb := range resources.ClusterRoleBindings {
		result.ClusterRoleBindings = append(result.ClusterRoleBindings, renderClusterRoleBindingBlock(crb))
	}

	return result
}

func groupWorkloads(resources *ClusterResources) (deps, dsets, stsets []AppGroup) {
	// Index services and ingresses by selector/name
	serviceBySelector := make(map[string]*corev1.Service)
	for i := range resources.Services {
		svc := &resources.Services[i]
		// Skip kubernetes default service
		if svc.Name == "kubernetes" {
			continue
		}
		if appName, ok := svc.Spec.Selector["app.kubernetes.io/name"]; ok {
			serviceBySelector[appName] = svc
		} else if appName, ok := svc.Spec.Selector["app"]; ok {
			serviceBySelector[appName] = svc
		} else {
			serviceBySelector[svc.Name] = svc
		}
	}

	ingressesByService := make(map[string][]*networkingv1.Ingress)
	for i := range resources.Ingresses {
		ing := &resources.Ingresses[i]
		seen := make(map[string]struct{})
		for _, rule := range ing.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}
			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service == nil {
					continue
				}
				svcName := path.Backend.Service.Name
				if _, dup := seen[svcName]; dup {
					continue
				}
				seen[svcName] = struct{}{}
				ingressesByService[svcName] = append(ingressesByService[svcName], ing)
			}
		}
	}

	findService := func(name string, labels map[string]string) *corev1.Service {
		if svc, ok := serviceBySelector[name]; ok {
			return svc
		}
		if appLabel, ok := labels["app.kubernetes.io/name"]; ok {
			if svc, ok := serviceBySelector[appLabel]; ok {
				return svc
			}
		}
		return nil
	}

	for i := range resources.Deployments {
		dep := &resources.Deployments[i]
		group := AppGroup{Name: dep.Name, Deployment: dep}
		group.Service = findService(dep.Name, dep.Labels)
		if group.Service != nil {
			group.Ingresses = ingressesByService[group.Service.Name]
		}
		deps = append(deps, group)
	}

	for i := range resources.DaemonSets {
		ds := &resources.DaemonSets[i]
		group := AppGroup{Name: ds.Name, DaemonSet: ds}
		group.Service = findService(ds.Name, ds.Labels)
		dsets = append(dsets, group)
	}

	for i := range resources.StatefulSets {
		sts := &resources.StatefulSets[i]
		group := AppGroup{Name: sts.Name, StatefulSet: sts}
		group.Service = findService(sts.Name, sts.Labels)
		// Also try the governing headless service by name
		if group.Service == nil && sts.Spec.ServiceName != "" {
			if svc, ok := serviceBySelector[sts.Spec.ServiceName]; ok {
				group.Service = svc
			}
		}
		if group.Service != nil {
			group.Ingresses = ingressesByService[group.Service.Name]
		}
		stsets = append(stsets, group)
	}

	return deps, dsets, stsets
}

func renderCronJobBlock(cj batchv1.CronJob) string {
	podSpec := cj.Spec.JobTemplate.Spec.Template.Spec
	container := podSpec.Containers[0]
	var b strings.Builder

	b.WriteString(fmt.Sprintf("cronjob %q {\n", cj.Name))

	if cj.Namespace != "" {
		b.WriteString(fmt.Sprintf("  namespace = %q\n", cj.Namespace))
	}
	b.WriteString(fmt.Sprintf("  schedule  = %q\n", cj.Spec.Schedule))
	b.WriteString(fmt.Sprintf("  image     = %q\n", container.Image))

	// Container name if different from cronjob name
	if container.Name != "" && container.Name != cj.Name {
		b.WriteString(fmt.Sprintf("  container_name = %q\n", container.Name))
	}

	if container.ImagePullPolicy != "" && container.ImagePullPolicy != "IfNotPresent" {
		b.WriteString(fmt.Sprintf("  image_pull_policy = %q\n", string(container.ImagePullPolicy)))
	}

	if len(podSpec.ImagePullSecrets) > 0 {
		var secrets []string
		for _, s := range podSpec.ImagePullSecrets {
			secrets = append(secrets, fmt.Sprintf("%q", s.Name))
		}
		b.WriteString(fmt.Sprintf("  image_pull_secrets = [%s]\n", strings.Join(secrets, ", ")))
	}

	// Command
	writeCommandMultiLine(&b, container.Command, "  ")

	// Concurrency
	if cj.Spec.ConcurrencyPolicy != "" && cj.Spec.ConcurrencyPolicy != batchv1.AllowConcurrent {
		b.WriteString(fmt.Sprintf("  concurrency = %q\n", string(cj.Spec.ConcurrencyPolicy)))
	}

	// Deadline
	if cj.Spec.StartingDeadlineSeconds != nil {
		b.WriteString(fmt.Sprintf("  deadline    = %q\n", formatDuration(*cj.Spec.StartingDeadlineSeconds)))
	}

	// Suspend
	if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
		b.WriteString("  suspend     = true\n")
	}

	// RestartPolicy
	if podSpec.RestartPolicy != "" && podSpec.RestartPolicy != corev1.RestartPolicyOnFailure {
		b.WriteString(fmt.Sprintf("  restart = %q\n", string(podSpec.RestartPolicy)))
	}

	// Env
	writeEnv(&b, container)

	// EnvFrom
	writeEnvFrom(&b, container)

	// Resources
	writeResources(&b, container)

	// Volumes
	writeVolumes(&b, podSpec.Volumes, container.VolumeMounts)

	// SecurityContext
	writeSecurityContext(&b, container.SecurityContext, podSpec.SecurityContext)

	writeHostPodFlags(&b, podSpec)
	writeTolerations(&b, podSpec.Tolerations)

	b.WriteString("}\n")
	return b.String()
}

// --- helpers ---

func writeProbeSettings(b *strings.Builder, probe *corev1.Probe, indent string) {
	if probe == nil {
		return
	}
	if probe.InitialDelaySeconds != 0 {
		b.WriteString(fmt.Sprintf("%sinitial_delay = %d\n", indent, probe.InitialDelaySeconds))
	}
	if probe.PeriodSeconds != 0 && probe.PeriodSeconds != 10 {
		b.WriteString(fmt.Sprintf("%speriod = %d\n", indent, probe.PeriodSeconds))
	}
	if probe.TimeoutSeconds != 0 && probe.TimeoutSeconds != 1 {
		b.WriteString(fmt.Sprintf("%stimeout = %d\n", indent, probe.TimeoutSeconds))
	}
	if probe.FailureThreshold != 0 && probe.FailureThreshold != 3 {
		b.WriteString(fmt.Sprintf("%sfailures = %d\n", indent, probe.FailureThreshold))
	}
	if probe.SuccessThreshold != 0 && probe.SuccessThreshold != 1 {
		b.WriteString(fmt.Sprintf("%ssuccesses = %d\n", indent, probe.SuccessThreshold))
	}
}

func writeCommandMultiLine(b *strings.Builder, command []string, indent string) {
	if len(command) == 0 {
		return
	}
	b.WriteString(fmt.Sprintf("%scommand = [\n", indent))
	for _, arg := range command {
		b.WriteString(fmt.Sprintf("%s  %q,\n", indent, arg))
	}
	b.WriteString(fmt.Sprintf("%s]\n", indent))
}

func writeTolerations(b *strings.Builder, tols []corev1.Toleration) {
	for _, t := range tols {
		b.WriteString("\n  toleration {\n")
		if t.Key != "" {
			b.WriteString(fmt.Sprintf("    key      = %q\n", t.Key))
		}
		if t.Operator != "" {
			b.WriteString(fmt.Sprintf("    operator = %q\n", string(t.Operator)))
		}
		if t.Value != "" {
			b.WriteString(fmt.Sprintf("    value    = %q\n", t.Value))
		}
		if t.Effect != "" {
			b.WriteString(fmt.Sprintf("    effect   = %q\n", string(t.Effect)))
		}
		if t.TolerationSeconds != nil {
			b.WriteString(fmt.Sprintf("    toleration_seconds = %d\n", *t.TolerationSeconds))
		}
		b.WriteString("  }\n")
	}
}

// writeHostPodFlags emits host_network / host_pid / host_ipc / dns_policy
// at workload-block indent. Only writes non-default values.
func writeHostPodFlags(b *strings.Builder, spec corev1.PodSpec) {
	if spec.HostNetwork {
		b.WriteString("  host_network = true\n")
	}
	if spec.HostPID {
		b.WriteString("  host_pid = true\n")
	}
	if spec.HostIPC {
		b.WriteString("  host_ipc = true\n")
	}
	if spec.DNSPolicy != "" && spec.DNSPolicy != corev1.DNSClusterFirst {
		b.WriteString(fmt.Sprintf("  dns_policy = %q\n", string(spec.DNSPolicy)))
	}
}

func writeNodeSelector(b *strings.Builder, ns map[string]string) {
	if len(ns) == 0 {
		return
	}
	b.WriteString("  node_selector = {\n")
	for _, k := range sortedKeys(ns) {
		b.WriteString(fmt.Sprintf("    %q = %q\n", k, ns[k]))
	}
	b.WriteString("  }\n")
}

func writeArgsMultiLine(b *strings.Builder, args []string, indent string) {
	if len(args) == 0 {
		return
	}
	b.WriteString(fmt.Sprintf("%sargs = [\n", indent))
	for _, a := range args {
		b.WriteString(fmt.Sprintf("%s  %q,\n", indent, a))
	}
	b.WriteString(fmt.Sprintf("%s]\n", indent))
}

func writeResources(b *strings.Builder, c corev1.Container, indent ...string) {
	ind := "  "
	if len(indent) > 0 {
		ind = indent[0]
	}
	if c.Resources.Requests == nil && c.Resources.Limits == nil {
		return
	}

	b.WriteString(fmt.Sprintf("\n%sresources {\n", ind))
	inner := ind + "  "
	if cpuReq := c.Resources.Requests.Cpu(); cpuReq != nil && !cpuReq.IsZero() {
		cpuLim := c.Resources.Limits.Cpu()
		if cpuLim != nil && !cpuLim.IsZero() && cpuReq.String() != cpuLim.String() {
			b.WriteString(fmt.Sprintf("%scpu    = %q\n", inner, cpuReq.String()+".."+cpuLim.String()))
		} else {
			b.WriteString(fmt.Sprintf("%scpu    = %q\n", inner, cpuReq.String()))
		}
	}
	if memReq := c.Resources.Requests.Memory(); memReq != nil && !memReq.IsZero() {
		memLim := c.Resources.Limits.Memory()
		if memLim != nil && !memLim.IsZero() && memReq.String() != memLim.String() {
			b.WriteString(fmt.Sprintf("%smemory = %q\n", inner, memReq.String()+".."+memLim.String()))
		} else {
			b.WriteString(fmt.Sprintf("%smemory = %q\n", inner, memReq.String()))
		}
	}
	if esReq := c.Resources.Requests.StorageEphemeral(); esReq != nil && !esReq.IsZero() {
		esLim := c.Resources.Limits.StorageEphemeral()
		if esLim != nil && !esLim.IsZero() && esReq.String() != esLim.String() {
			b.WriteString(fmt.Sprintf("%sephemeral_storage = %q\n", inner, esReq.String()+".."+esLim.String()))
		} else {
			b.WriteString(fmt.Sprintf("%sephemeral_storage = %q\n", inner, esReq.String()))
		}
	}
	b.WriteString(fmt.Sprintf("%s}\n", ind))
}

func writeEnv(b *strings.Builder, c corev1.Container, indent ...string) {
	ind := "  "
	if len(indent) > 0 {
		ind = indent[0]
	}
	if len(c.Env) == 0 {
		return
	}

	inner := ind + "  "
	b.WriteString(fmt.Sprintf("\n%senv {\n", ind))
	for _, e := range c.Env {
		switch {
		case e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil:
			b.WriteString(fmt.Sprintf("%s%s = secret(%q, %q)\n",
				inner, e.Name, e.ValueFrom.SecretKeyRef.Name, e.ValueFrom.SecretKeyRef.Key))
		case e.ValueFrom != nil && e.ValueFrom.ConfigMapKeyRef != nil:
			b.WriteString(fmt.Sprintf("%s%s = configmap(%q, %q)\n",
				inner, e.Name, e.ValueFrom.ConfigMapKeyRef.Name, e.ValueFrom.ConfigMapKeyRef.Key))
		case e.ValueFrom != nil && e.ValueFrom.FieldRef != nil:
			b.WriteString(fmt.Sprintf("%s%s = field_ref(%q)\n",
				inner, e.Name, e.ValueFrom.FieldRef.FieldPath))
		default:
			b.WriteString(fmt.Sprintf("%s%s = %q\n", inner, e.Name, e.Value))
		}
	}
	b.WriteString(fmt.Sprintf("%s}\n", ind))
}

func writeEnvFrom(b *strings.Builder, c corev1.Container, indent ...string) {
	ind := "  "
	if len(indent) > 0 {
		ind = indent[0]
	}
	inner := ind + "  "
	for _, ef := range c.EnvFrom {
		b.WriteString(fmt.Sprintf("\n%senv_from {\n", ind))
		if ef.ConfigMapRef != nil {
			b.WriteString(fmt.Sprintf("%sconfig_map = %q\n", inner, ef.ConfigMapRef.Name))
		}
		if ef.SecretRef != nil {
			b.WriteString(fmt.Sprintf("%ssecret = %q\n", inner, ef.SecretRef.Name))
		}
		if ef.Prefix != "" {
			b.WriteString(fmt.Sprintf("%sprefix = %q\n", inner, ef.Prefix))
		}
		b.WriteString(fmt.Sprintf("%s}\n", ind))
	}
}

func writeIngress(b *strings.Builder, ing *networkingv1.Ingress, group AppGroup) {
	appName := group.Name
	if len(ing.Spec.Rules) == 0 {
		return
	}

	// Collect all unique hosts from rules
	var hosts []string
	for _, rule := range ing.Spec.Rules {
		if rule.Host != "" {
			found := false
			for _, h := range hosts {
				if h == rule.Host {
					found = true
					break
				}
			}
			if !found {
				hosts = append(hosts, rule.Host)
			}
		}
	}

	b.WriteString("\n  ingress {\n")
	if ing.Name != appName {
		b.WriteString(fmt.Sprintf("    name = %q\n", ing.Name))
	}

	// Capture backend service name if different from app name
	if len(ing.Spec.Rules) > 0 && ing.Spec.Rules[0].HTTP != nil && len(ing.Spec.Rules[0].HTTP.Paths) > 0 {
		backend := ing.Spec.Rules[0].HTTP.Paths[0].Backend
		if backend.Service != nil && backend.Service.Name != appName {
			b.WriteString(fmt.Sprintf("    service_name = %q\n", backend.Service.Name))
		}
		// Capture backend port if it differs from the app's first port
		if backend.Service != nil && backend.Service.Port.Number > 0 {
			// Check if it matches the app's first container port
			appPort := int32(0)
			if group.Deployment != nil && len(group.Deployment.Spec.Template.Spec.Containers) > 0 {
				ports := group.Deployment.Spec.Template.Spec.Containers[0].Ports
				if len(ports) > 0 {
					appPort = ports[0].ContainerPort
				}
			}
			if backend.Service.Port.Number != appPort {
				b.WriteString(fmt.Sprintf("    port = %d\n", backend.Service.Port.Number))
			}
		}
	}

	// Sort hosts for deterministic output
	sort.Strings(hosts)

	if len(hosts) == 1 {
		b.WriteString(fmt.Sprintf("    host = %q\n", hosts[0]))
	} else if len(hosts) > 1 {
		b.WriteString("    hosts = [\n")
		for _, h := range hosts {
			b.WriteString(fmt.Sprintf("      %q,\n", h))
		}
		b.WriteString("    ]\n")
	}

	if len(ing.Spec.TLS) > 0 {
		b.WriteString("    tls  = true\n")
		if ing.Spec.TLS[0].SecretName != "" {
			b.WriteString(fmt.Sprintf("    tls_secret = %q\n", ing.Spec.TLS[0].SecretName))
		}
	}

	// Annotations — group by common prefix
	if len(ing.Annotations) > 0 {
		b.WriteString("\n    annotations = {\n")
		writeGroupedAnnotations(b, ing.Annotations, "      ")
		b.WriteString("    }\n")
	}

	b.WriteString("  }\n")
}

// writeGroupedAnnotations groups annotations by their prefix (everything before the last /).
// E.g. "nginx.ingress.kubernetes.io/enable-cors" groups under "nginx.ingress.kubernetes.io".
func writeGroupedAnnotations(b *strings.Builder, annotations map[string]string, indent string) {
	// Group by prefix
	groups := make(map[string]map[string]string) // prefix -> suffix -> value
	ungrouped := make(map[string]string)          // keys without a groupable prefix

	for _, k := range sortedKeys(annotations) {
		lastSlash := strings.LastIndex(k, "/")
		if lastSlash == -1 {
			ungrouped[k] = annotations[k]
			continue
		}
		prefix := k[:lastSlash]
		suffix := k[lastSlash+1:]

		if groups[prefix] == nil {
			groups[prefix] = make(map[string]string)
		}
		groups[prefix][suffix] = annotations[k]
	}

	// Write ungrouped annotations first
	for _, k := range sortedKeys(ungrouped) {
		b.WriteString(fmt.Sprintf("%s%q = %q\n", indent, k, ungrouped[k]))
	}

	// Write grouped annotations
	var prefixes []string
	for p := range groups {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)

	for _, prefix := range prefixes {
		suffixes := groups[prefix]
		if len(suffixes) == 1 {
			// Single annotation in group — write flat
			for suffix, val := range suffixes {
				b.WriteString(fmt.Sprintf("%s%q = %q\n", indent, prefix+"/"+suffix, val))
			}
		} else {
			// Multiple annotations — write grouped
			b.WriteString(fmt.Sprintf("%s%q = {\n", indent, prefix))
			for _, suffix := range sortedKeys(suffixes) {
				b.WriteString(fmt.Sprintf("%s  %q = %q\n", indent, suffix, suffixes[suffix]))
			}
			b.WriteString(fmt.Sprintf("%s}\n", indent))
		}
	}
}

func renderConfigMapBlock(cm corev1.ConfigMap) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("configmap %q {\n", cm.Name))

	if cm.Namespace != "" {
		b.WriteString(fmt.Sprintf("  namespace = %q\n", cm.Namespace))
	}

	if len(cm.Data) > 0 {
		b.WriteString("\n  data = {\n")
		for _, k := range sortedKeys(cm.Data) {
			v := cm.Data[k]
			if strings.Contains(v, "\n") {
				// Multi-line: suggest using file() — write as comment + placeholder
				b.WriteString(fmt.Sprintf("    # Multi-line value — consider: %q = file(\"configs/%s\")\n", k, k))
				b.WriteString(fmt.Sprintf("    %q = %q\n", k, v))
			} else {
				b.WriteString(fmt.Sprintf("    %q = %q\n", k, v))
			}
		}
		b.WriteString("  }\n")
	}

	b.WriteString("}\n")
	return b.String()
}

func writeVolumes(b *strings.Builder, volumes []corev1.Volume, mounts []corev1.VolumeMount, indent ...string) {
	ind := "  "
	if len(indent) > 0 {
		ind = indent[0]
	}
	inner := ind + "  "

	mountByName := make(map[string]corev1.VolumeMount)
	for _, m := range mounts {
		mountByName[m.Name] = m
	}

	for _, vol := range volumes {
		mount, hasMnt := mountByName[vol.Name]
		if !hasMnt {
			continue
		}

		b.WriteString(fmt.Sprintf("\n%svolume %q {\n", ind, vol.Name))
		b.WriteString(fmt.Sprintf("%smount_path = %q\n", inner, mount.MountPath))

		if mount.SubPath != "" {
			b.WriteString(fmt.Sprintf("%ssub_path   = %q\n", inner, mount.SubPath))
		}
		if mount.ReadOnly {
			b.WriteString(fmt.Sprintf("%sread_only  = true\n", inner))
		}

		switch {
		case vol.Secret != nil:
			b.WriteString(fmt.Sprintf("%ssecret = %q\n", inner, vol.Secret.SecretName))
		case vol.ConfigMap != nil:
			b.WriteString(fmt.Sprintf("%sconfig_map = %q\n", inner, vol.ConfigMap.Name))
		case vol.EmptyDir != nil:
			b.WriteString(fmt.Sprintf("%sempty_dir = true\n", inner))
		case vol.PersistentVolumeClaim != nil:
			b.WriteString(fmt.Sprintf("%spvc = %q\n", inner, vol.PersistentVolumeClaim.ClaimName))
		case vol.HostPath != nil:
			b.WriteString(fmt.Sprintf("%shost_path = %q\n", inner, vol.HostPath.Path))
			if vol.HostPath.Type != nil && *vol.HostPath.Type != "" {
				b.WriteString(fmt.Sprintf("%shost_path_type = %q\n", inner, string(*vol.HostPath.Type)))
			}
		}

		b.WriteString(fmt.Sprintf("%s}\n", ind))
	}
}

func writeSecurityContext(b *strings.Builder, containerSC *corev1.SecurityContext, podSC *corev1.PodSecurityContext, indent ...string) {
	ind := "  "
	if len(indent) > 0 {
		ind = indent[0]
	}
	inner := ind + "  "

	if containerSC == nil && podSC == nil {
		return
	}

	hasContent := false
	var sc strings.Builder

	if containerSC != nil {
		if containerSC.RunAsUser != nil {
			sc.WriteString(fmt.Sprintf("%srun_as_user     = %d\n", inner, *containerSC.RunAsUser))
			hasContent = true
		}
		if containerSC.RunAsGroup != nil {
			sc.WriteString(fmt.Sprintf("%srun_as_group    = %d\n", inner, *containerSC.RunAsGroup))
			hasContent = true
		}
		if containerSC.RunAsNonRoot != nil && *containerSC.RunAsNonRoot {
			sc.WriteString(fmt.Sprintf("%srun_as_non_root = true\n", inner))
			hasContent = true
		}
		if containerSC.ReadOnlyRootFilesystem != nil && *containerSC.ReadOnlyRootFilesystem {
			sc.WriteString(fmt.Sprintf("%sread_only_root  = true\n", inner))
			hasContent = true
		}
		if containerSC.Privileged != nil && *containerSC.Privileged {
			sc.WriteString(fmt.Sprintf("%sprivileged      = true\n", inner))
			hasContent = true
		}
	}
	if podSC != nil && podSC.FSGroup != nil {
		sc.WriteString(fmt.Sprintf("%sfs_group        = %d\n", inner, *podSC.FSGroup))
		hasContent = true
	}

	if hasContent {
		b.WriteString(fmt.Sprintf("\n%ssecurity_context {\n", ind))
		b.WriteString(sc.String())
		b.WriteString(fmt.Sprintf("%s}\n", ind))
	}
}

func formatDuration(seconds int64) string {
	if seconds%3600 == 0 {
		return fmt.Sprintf("%dh", seconds/3600)
	}
	if seconds%60 == 0 {
		return fmt.Sprintf("%dm", seconds/60)
	}
	return fmt.Sprintf("%ds", seconds)
}

func renderPersistentVolumeClaimBlock(pvc corev1.PersistentVolumeClaim) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("persistentvolumeclaim %q {\n", pvc.Name))

	if pvc.Namespace != "" {
		b.WriteString(fmt.Sprintf("  namespace = %q\n", pvc.Namespace))
	}

	if pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName != "" {
		b.WriteString(fmt.Sprintf("  storage_class = %q\n", *pvc.Spec.StorageClassName))
	}

	if len(pvc.Spec.AccessModes) > 0 {
		var modes []string
		for _, m := range pvc.Spec.AccessModes {
			modes = append(modes, fmt.Sprintf("%q", string(m)))
		}
		b.WriteString(fmt.Sprintf("  access_modes  = [%s]\n", strings.Join(modes, ", ")))
	}

	if storage, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
		b.WriteString(fmt.Sprintf("  storage       = %q\n", storage.String()))
	}

	b.WriteString("}\n")
	return b.String()
}

// sortedKeys returns sorted keys of a map.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
