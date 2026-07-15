package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/allcallall/backend/internal/mcpplatform"
)

const (
	defaultSandboxJobStartupTimeout = 30 * time.Second
	defaultSandboxJobPollInterval   = 250 * time.Millisecond
	defaultSandboxJobRunnerPort     = 8093
)

type KubernetesRunnerConfig struct {
	Namespace        string
	RunnerImage      string
	SupervisorImage  string
	RuntimeClass     string
	ServiceAccount   string
	AppName          string
	Instance         string
	OpenBaoAddress   string
	ImagePullSecrets []string
	CPU              string
	Memory           string
	StartupTimeout   time.Duration
	PollInterval     time.Duration
}

// KubernetesRunner starts each OCI installation in its own short-lived pod.
// The request, including its one-time wrapping token, is sent directly to the
// ready pod and is never written into the Job object.
type KubernetesRunner struct {
	client         kubernetes.Interface
	config         KubernetesRunnerConfig
	httpClient     *http.Client
	endpointForPod func(string) string
}

type kubernetesPreparedExecution struct {
	runner   *KubernetesRunner
	jobName  string
	endpoint string
	request  mcpplatform.ExecutionRequest
}

func NewInClusterKubernetesRunner(config KubernetesRunnerConfig) (*KubernetesRunner, error) {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("load in-cluster Kubernetes config: %w", err)
	}
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("initialize Kubernetes client: %w", err)
	}
	return NewKubernetesRunner(client, config)
}

func NewKubernetesRunner(client kubernetes.Interface, config KubernetesRunnerConfig) (*KubernetesRunner, error) {
	config.Namespace = strings.TrimSpace(config.Namespace)
	config.RunnerImage = strings.TrimSpace(config.RunnerImage)
	config.SupervisorImage = strings.TrimSpace(config.SupervisorImage)
	config.RuntimeClass = strings.TrimSpace(config.RuntimeClass)
	config.ServiceAccount = strings.TrimSpace(config.ServiceAccount)
	config.AppName = strings.TrimSpace(config.AppName)
	config.Instance = strings.TrimSpace(config.Instance)
	config.OpenBaoAddress = strings.TrimSpace(config.OpenBaoAddress)
	for index, name := range config.ImagePullSecrets {
		config.ImagePullSecrets[index] = strings.TrimSpace(name)
		if config.ImagePullSecrets[index] == "" {
			return nil, fmt.Errorf("Kubernetes sandbox image pull secret name is empty")
		}
	}
	if client == nil || config.Namespace == "" || config.RunnerImage == "" || config.SupervisorImage == "" || config.RuntimeClass == "" || config.ServiceAccount == "" || config.AppName == "" || config.Instance == "" {
		return nil, fmt.Errorf("Kubernetes sandbox runner configuration is incomplete")
	}
	if config.CPU == "" {
		config.CPU = "500m"
	}
	if config.Memory == "" {
		config.Memory = "512Mi"
	}
	if _, err := resource.ParseQuantity(config.CPU); err != nil {
		return nil, fmt.Errorf("invalid sandbox CPU limit: %w", err)
	}
	if _, err := resource.ParseQuantity(config.Memory); err != nil {
		return nil, fmt.Errorf("invalid sandbox memory limit: %w", err)
	}
	if config.StartupTimeout <= 0 {
		config.StartupTimeout = defaultSandboxJobStartupTimeout
	}
	if config.PollInterval <= 0 {
		config.PollInterval = defaultSandboxJobPollInterval
	}
	runner := &KubernetesRunner{
		client: client,
		config: config,
		httpClient: &http.Client{
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
	runner.endpointForPod = func(podIP string) string {
		return "http://" + net.JoinHostPort(podIP, fmt.Sprintf("%d", defaultSandboxJobRunnerPort))
	}
	return runner, nil
}

func (r *KubernetesRunner) Validate(ctx context.Context, request mcpplatform.ValidationRequest) (mcpplatform.ValidationResult, error) {
	job, endpoint, err := r.preparePod(ctx, request.Definition, "validate", "")
	if err != nil {
		return mcpplatform.ValidationResult{}, err
	}
	defer r.deleteJob(job)
	var result mcpplatform.ValidationResult
	if err := r.post(ctx, endpoint+"/v1/validate", request, &result); err != nil {
		return mcpplatform.ValidationResult{}, err
	}
	return result, nil
}

func (r *KubernetesRunner) Execute(ctx context.Context, request mcpplatform.ExecutionRequest) (mcpplatform.ExecutionResult, error) {
	prepared, err := r.PrepareExecution(ctx, request)
	if err != nil {
		return mcpplatform.ExecutionResult{}, err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = prepared.Close(closeCtx)
	}()
	return prepared.Execute(ctx)
}

func (r *KubernetesRunner) PrepareExecution(ctx context.Context, request mcpplatform.ExecutionRequest) (PreparedExecution, error) {
	job, endpoint, err := r.preparePod(ctx, request.Definition, "execute", request.ExecutionID)
	if err != nil {
		return nil, err
	}
	return &kubernetesPreparedExecution{
		runner:   r,
		jobName:  job,
		endpoint: endpoint,
		request:  request,
	}, nil
}

func (p *kubernetesPreparedExecution) JobID() string { return p.jobName }

func (p *kubernetesPreparedExecution) Execute(ctx context.Context) (mcpplatform.ExecutionResult, error) {
	var result mcpplatform.ExecutionResult
	if err := p.runner.post(ctx, p.endpoint+"/v1/execute", p.request, &result); err != nil {
		return mcpplatform.ExecutionResult{JobID: p.jobName}, err
	}
	result.JobID = p.jobName
	return result, nil
}

func (p *kubernetesPreparedExecution) Close(ctx context.Context) error {
	return p.runner.deleteJobWithContext(ctx, p.jobName)
}

func (r *KubernetesRunner) preparePod(ctx context.Context, definition mcpplatform.InstallationDefinition, operation, executionID string) (string, string, error) {
	if err := validateDigestPinned(definition.ImageRef); err != nil {
		return "", "", err
	}
	job := r.buildJob(definition.ImageRef, operation, executionID)
	created, err := r.client.BatchV1().Jobs(r.config.Namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return "", "", fmt.Errorf("create isolated sandbox Job: %w", err)
	}
	jobName := created.Name
	waitCtx, cancel := context.WithTimeout(ctx, r.config.StartupTimeout)
	defer cancel()
	podIP, err := r.waitForRunner(waitCtx, jobName)
	if err != nil {
		r.deleteJob(jobName)
		return "", "", err
	}
	return jobName, r.endpointForPod(podIP), nil
}

func (r *KubernetesRunner) buildJob(imageRef, operation, executionID string) *batchv1.Job {
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
	for _, name := range r.config.ImagePullSecrets {
		job.Spec.Template.Spec.ImagePullSecrets = append(job.Spec.Template.Spec.ImagePullSecrets, corev1.LocalObjectReference{Name: name})
	}
	return job
}

func (r *KubernetesRunner) waitForRunner(ctx context.Context, jobName string) (string, error) {
	selector := labels.SelectorFromSet(map[string]string{"job-name": jobName}).String()
	ticker := time.NewTicker(r.config.PollInterval)
	defer ticker.Stop()
	for {
		pods, err := r.client.CoreV1().Pods(r.config.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			return "", fmt.Errorf("list isolated sandbox pods: %w", err)
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodFailed {
				return "", errors.New("isolated sandbox pod failed before startup")
			}
			statuses := append(append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...), pod.Status.ContainerStatuses...)
			for _, status := range statuses {
				if status.State.Terminated != nil || fatalContainerWait(status.State.Waiting) {
					return "", errors.New("isolated sandbox container failed before startup")
				}
				if status.Name == "runner" && status.Ready && strings.TrimSpace(pod.Status.PodIP) != "" {
					return pod.Status.PodIP, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("wait for isolated sandbox pod: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func fatalContainerWait(waiting *corev1.ContainerStateWaiting) bool {
	if waiting == nil {
		return false
	}
	switch waiting.Reason {
	case "CreateContainerConfigError", "CreateContainerError", "ErrImagePull", "ImagePullBackOff", "InvalidImageName", "RunContainerError":
		return true
	default:
		return false
	}
}

func (r *KubernetesRunner) post(ctx context.Context, endpoint string, input, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode isolated Runner request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build isolated Runner request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := r.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call isolated Runner: %w", err)
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, int64(mcpplatform.DefaultOutputLimit+64*1024)+1)
	responseBody, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read isolated Runner response: %w", err)
	}
	if len(responseBody) > mcpplatform.DefaultOutputLimit+64*1024 {
		return mcpplatform.ErrOutputTooLarge
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("isolated Runner returned status %d", response.StatusCode)
	}
	if err := json.Unmarshal(responseBody, output); err != nil {
		return fmt.Errorf("decode isolated Runner response: %w", err)
	}
	return nil
}

func (r *KubernetesRunner) deleteJob(jobName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = r.deleteJobWithContext(ctx, jobName)
}

func (r *KubernetesRunner) deleteJobWithContext(ctx context.Context, jobName string) error {
	propagation := metav1.DeletePropagationBackground
	err := r.client.BatchV1().Jobs(r.config.Namespace).Delete(ctx, jobName, metav1.DeleteOptions{PropagationPolicy: &propagation})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete isolated sandbox Job: %w", err)
	}
	return nil
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
