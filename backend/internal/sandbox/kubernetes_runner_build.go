package sandbox

import (
	"github.com/google/uuid"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	"strings"
	"time"
)

func (r *KubernetesRunner) buildJob(imageRef, operation, executionID string, destinations []pinnedNetworkDestination) *batchv1.Job {
	zero := int32(0)
	ttl := int32(300)
	deadline := int64((r.config.StartupTimeout + 35*time.Second + time.Second - 1) / time.Second)
	runtimeClass := r.config.RuntimeClass
	uid := int64(10001)
	readOnly := true
	allowPrivilegeEscalation := false
	runAsNonRoot := true
	labels := map[string]string{
		"app.kubernetes.io/name":       r.config.AppName,
		"app.kubernetes.io/instance":   r.config.Instance,
		"app.kubernetes.io/component":  "sandbox-job",
		"app.kubernetes.io/managed-by": "sandbox-control-plane",
		"allcallall.io/sandbox-id":     uuid.NewString(),
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "allcallall-mcp-",
			Namespace:    r.config.Namespace,
			Labels:       labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &zero,
			TTLSecondsAfterFinished: &ttl,
			ActiveDeadlineSeconds:   &deadline,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken:  ptr(false),
					ServiceAccountName:            r.config.ServiceAccount,
					RuntimeClassName:              &runtimeClass,
					RestartPolicy:                 corev1.RestartPolicyNever,
					EnableServiceLinks:            ptr(false),
					TerminationGracePeriodSeconds: ptr(int64(5)),
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot:   &runAsNonRoot,
						RunAsUser:      &uid,
						RunAsGroup:     &uid,
						FSGroup:        &uid,
						SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
					InitContainers: []corev1.Container{{
						Name:            "supervisor-installer",
						Image:           r.config.SupervisorImage,
						Args:            []string{"install", "/opt/allcallall/sandbox-supervisor"},
						SecurityContext: lockedContainerSecurityContext(uid, readOnly, allowPrivilegeEscalation, runAsNonRoot),
						VolumeMounts:    []corev1.VolumeMount{{Name: "supervisor-bin", MountPath: "/opt/allcallall"}},
					}},
					Containers: []corev1.Container{
						{
							Name:            "mcp-server",
							Image:           imageRef,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/opt/allcallall/sandbox-supervisor"},
							Args:            []string{"serve", "--socket", "/run/allcallall/supervisor.sock"},
							SecurityContext: lockedContainerSecurityContext(uid, readOnly, allowPrivilegeEscalation, runAsNonRoot),
							Resources:       r.toolResources(),
							VolumeMounts: []corev1.VolumeMount{
								{Name: "supervisor-bin", MountPath: "/opt/allcallall", ReadOnly: true},
								{Name: "runtime", MountPath: "/run/allcallall"},
								{Name: "tool-tmp", MountPath: "/tmp"},
							},
						},
						{
							Name:            "runner",
							Image:           r.config.RunnerImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Env: []corev1.EnvVar{
								{Name: "SANDBOX_SUPERVISOR_SOCKET", Value: "/run/allcallall/supervisor.sock"},
								{Name: "SANDBOX_ALLOW_STDIO", Value: "0"},
								{Name: "SANDBOX_ONE_SHOT", Value: "1"},
								{Name: "SANDBOX_OPERATION", Value: operation},
								{Name: "SANDBOX_EXPECTED_EXECUTION_ID", Value: executionID},
								{Name: "OPENBAO_ADDR", Value: r.config.OpenBaoAddress},
							},
							Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: defaultSandboxJobRunnerPort}},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler:     corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromString("http")}},
								PeriodSeconds:    1,
								FailureThreshold: 30,
							},
							SecurityContext: lockedContainerSecurityContext(uid, readOnly, allowPrivilegeEscalation, runAsNonRoot),
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("96Mi")},
								Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m"), corev1.ResourceMemory: resource.MustParse("256Mi")},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "runtime", MountPath: "/run/allcallall"},
								{Name: "runner-tmp", MountPath: "/tmp"},
							},
						},
					},
					Volumes: []corev1.Volume{
						{Name: "supervisor-bin", VolumeSource: memoryVolume("16Mi")},
						{Name: "runtime", VolumeSource: memoryVolume("2Mi")},
						{Name: "tool-tmp", VolumeSource: memoryVolume("64Mi")},
						{Name: "runner-tmp", VolumeSource: memoryVolume("64Mi")},
					},
				},
			},
		},
	}
	for _, destination := range destinations {
		for _, address := range destination.addresses {
			job.Spec.Template.Spec.HostAliases = append(job.Spec.Template.Spec.HostAliases, corev1.HostAlias{IP: address, Hostnames: []string{destination.hostname}})
		}
	}
	for _, name := range r.config.ImagePullSecrets {
		job.Spec.Template.Spec.ImagePullSecrets = append(job.Spec.Template.Spec.ImagePullSecrets, corev1.LocalObjectReference{Name: name})
	}
	return job
}

func (r *KubernetesRunner) buildNetworkPolicy(job *batchv1.Job, destinations []pinnedNetworkDestination) *networkingv1.NetworkPolicy {
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      job.Name + "-egress",
			Namespace: r.config.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       r.config.AppName,
				"app.kubernetes.io/instance":   r.config.Instance,
				"app.kubernetes.io/component":  "sandbox-job-egress",
				"app.kubernetes.io/managed-by": "sandbox-control-plane",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{
				"allcallall.io/sandbox-id": job.Spec.Template.Labels["allcallall.io/sandbox-id"],
			}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress:      []networkingv1.NetworkPolicyEgressRule{},
		},
	}
	if job.UID != "" {
		policy.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: batchv1.SchemeGroupVersion.String(),
			Kind:       "Job",
			Name:       job.Name,
			UID:        job.UID,
		}}
	}
	peers := make([]networkingv1.NetworkPolicyPeer, 0)
	for _, destination := range destinations {
		for _, address := range destination.addresses {
			cidr := address + "/32"
			if strings.Contains(address, ":") {
				cidr = address + "/128"
			}
			peers = append(peers, networkingv1.NetworkPolicyPeer{IPBlock: &networkingv1.IPBlock{CIDR: cidr}})
		}
	}
	if len(peers) > 0 {
		protocol := corev1.ProtocolTCP
		port := intstr.FromInt32(443)
		policy.Spec.Egress = append(policy.Spec.Egress, networkingv1.NetworkPolicyEgressRule{
			To:    peers,
			Ports: []networkingv1.NetworkPolicyPort{{Protocol: &protocol, Port: &port}},
		})
	}
	return policy
}

func (r *KubernetesRunner) toolResources() corev1.ResourceRequirements {
	cpu := resource.MustParse(r.config.CPU)
	memory := resource.MustParse(r.config.Memory)
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: cpu, corev1.ResourceMemory: memory},
		Limits:   corev1.ResourceList{corev1.ResourceCPU: cpu, corev1.ResourceMemory: memory},
	}
}

func lockedContainerSecurityContext(uid int64, readOnly, allowPrivilegeEscalation, runAsNonRoot bool) *corev1.SecurityContext {
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: &allowPrivilegeEscalation,
		ReadOnlyRootFilesystem:   &readOnly,
		RunAsNonRoot:             &runAsNonRoot,
		RunAsUser:                &uid,
		RunAsGroup:               &uid,
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}
}

func memoryVolume(size string) corev1.VolumeSource {
	medium := corev1.StorageMediumMemory
	quantity := resource.MustParse(size)
	return corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{Medium: medium, SizeLimit: &quantity}}
}

func ptr[T any](value T) *T { return &value }
