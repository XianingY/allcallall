package sandbox

import (
	"fmt"
	"github.com/allcallall/backend/internal/mcpplatform"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net"
	"net/http"
	"strings"
	"time"
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
	resolver       ipResolver
}

type kubernetesPreparedExecution struct {
	runner        *KubernetesRunner
	jobName       string
	networkPolicy string
	endpoint      string
	request       mcpplatform.ExecutionRequest
}

type pinnedNetworkDestination struct {
	hostname  string
	addresses []string
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
		client:   client,
		config:   config,
		resolver: net.DefaultResolver,
		httpClient: &http.Client{
			Timeout: 35 * time.Second,
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
