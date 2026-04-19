package generator

import (
	"strconv"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gsid-nl/kdef/internal/types"
)

// GenerateCronJob creates a CronJob from a CronJobConfig.
func GenerateCronJob(cj types.CronJobConfig) *batchv1.CronJob {
	labels := kdefLabels(map[string]string{
		"app.kubernetes.io/name":      cj.Name,
		"app.kubernetes.io/component": "cronjob",
	})

	containerName := cj.Name
	if cj.ContainerName != "" {
		containerName = cj.ContainerName
	}

	container := corev1.Container{
		Name:  containerName,
		Image: cj.Image,
	}

	if cj.ImagePullPolicy != "" {
		container.ImagePullPolicy = corev1.PullPolicy(cj.ImagePullPolicy)
	}
	if len(cj.Command) > 0 {
		container.Command = cj.Command
	}
	if len(cj.Args) > 0 {
		container.Args = cj.Args
	}

	for _, e := range cj.Env {
		container.Env = append(container.Env, buildEnvVar(e))
	}
	for _, ef := range cj.EnvFrom {
		container.EnvFrom = append(container.EnvFrom, buildEnvFromSource(ef))
	}

	if cj.Resources != nil {
		container.Resources = buildResources(cj.Resources)
	}

	for _, v := range cj.Volumes {
		container.VolumeMounts = append(container.VolumeMounts, buildVolumeMount(v))
	}

	if cj.SecurityContext != nil {
		container.SecurityContext = buildSecurityContext(cj.SecurityContext)
	}

	volumes := buildVolumes(cj.Volumes)

	var imagePullSecrets []corev1.LocalObjectReference
	for _, s := range cj.ImagePullSecrets {
		imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{Name: s})
	}

	restartPolicy := corev1.RestartPolicyOnFailure
	if cj.Restart == "Never" {
		restartPolicy = corev1.RestartPolicyNever
	}

	spec := corev1.PodSpec{
		Containers:    []corev1.Container{container},
		RestartPolicy: restartPolicy,
	}
	if len(volumes) > 0 {
		spec.Volumes = volumes
	}
	if len(imagePullSecrets) > 0 {
		spec.ImagePullSecrets = imagePullSecrets
	}
	if cj.ServiceAccountName != "" {
		spec.ServiceAccountName = cj.ServiceAccountName
	}
	if len(cj.NodeSelector) > 0 {
		spec.NodeSelector = cj.NodeSelector
	}
	if tols := buildTolerations(cj.Tolerations); len(tols) > 0 {
		spec.Tolerations = tols
	}
	if cj.HostNetwork {
		spec.HostNetwork = true
	}
	if cj.HostPID {
		spec.HostPID = true
	}
	if cj.HostIPC {
		spec.HostIPC = true
	}
	if cj.DNSPolicy != "" {
		spec.DNSPolicy = corev1.DNSPolicy(cj.DNSPolicy)
	}

	concurrencyPolicy := batchv1.AllowConcurrent
	switch cj.Concurrency {
	case "Forbid":
		concurrencyPolicy = batchv1.ForbidConcurrent
	case "Replace":
		concurrencyPolicy = batchv1.ReplaceConcurrent
	}

	cronJob := &batchv1.CronJob{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "CronJob",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cj.Name,
			Namespace: cj.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          cj.Schedule,
			ConcurrencyPolicy: concurrencyPolicy,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: labels,
						},
						Spec: spec,
					},
				},
			},
		},
	}

	if cj.Deadline != "" {
		seconds := parseDuration(cj.Deadline)
		if seconds > 0 {
			cronJob.Spec.StartingDeadlineSeconds = &seconds
		}
	}

	if cj.Suspend != nil {
		cronJob.Spec.Suspend = cj.Suspend
	}

	return cronJob
}

func parseDuration(s string) int64 {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return 0
	}
	unit := s[len(s)-1]
	numStr := s[:len(s)-1]
	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0
	}
	switch unit {
	case 's':
		return num
	case 'm':
		return num * 60
	case 'h':
		return num * 3600
	default:
		return 0
	}
}
