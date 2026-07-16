package sandbox

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/allcallall/backend/internal/mcpplatform"
)

type staticIPResolver map[string][]net.IPAddr

func (resolver staticIPResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	return resolver[host], nil
}

func TestKubernetesJobIsolationContract(t *testing.T) {
	runner, err := NewKubernetesRunner(fake.NewSimpleClientset(), KubernetesRunnerConfig{
		Namespace:        "allcallall-sandbox",
		RunnerImage:      "registry.example.com/allcallall/runner:v1",
		SupervisorImage:  "registry.example.com/allcallall/supervisor:v1",
		RuntimeClass:     "gvisor",
		ServiceAccount:   "sandbox-job",
		AppName:          "allcallall",
		Instance:         "test",
		ImagePullSecrets: []string{"private-registry"},
		CPU:              "500m",
		Memory:           "512Mi",
		StartupTimeout:   time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	image := "registry.example.com/tool@sha256:" + strings.Repeat("a", 64)
	destinations := []pinnedNetworkDestination{{hostname: "api.example.com", addresses: []string{"8.8.8.8", "2606:4700:4700::1111"}}}
	job := runner.buildJob(image, "execute", "mcp:run-1:call-1", destinations)
	job.Name = "allcallall-mcp-contract"
	pod := job.Spec.Template.Spec
	if pod.RuntimeClassName == nil || *pod.RuntimeClassName != "gvisor" {
		t.Fatalf("runtime class=%v", pod.RuntimeClassName)
	}
	if pod.AutomountServiceAccountToken == nil || *pod.AutomountServiceAccountToken {
		t.Fatal("untrusted sandbox pod mounted a Kubernetes service account token")
	}
	if pod.ServiceAccountName != "sandbox-job" || len(pod.InitContainers) != 1 || len(pod.Containers) != 2 {
		t.Fatalf("unexpected sandbox pod topology: serviceAccount=%q init=%d containers=%d", pod.ServiceAccountName, len(pod.InitContainers), len(pod.Containers))
	}
	if len(pod.ImagePullSecrets) != 1 || pod.ImagePullSecrets[0].Name != "private-registry" {
		t.Fatalf("image pull secrets=%v", pod.ImagePullSecrets)
	}
	tool := pod.Containers[0]
	if tool.Name != "mcp-server" || tool.Image != image || strings.Join(tool.Command, " ") != "/opt/allcallall/sandbox-supervisor" {
		t.Fatalf("untrusted container does not use the digest-pinned supervisor contract: %#v", tool)
	}
	assertLockedContainer(t, tool)
	assertLockedContainer(t, pod.Containers[1])
	assertLockedContainer(t, pod.InitContainers[0])
	if pod.Containers[1].Env[4].Name != "SANDBOX_EXPECTED_EXECUTION_ID" || pod.Containers[1].Env[4].Value != "mcp:run-1:call-1" {
		t.Fatalf("Runner is not bound to the execution identity: %#v", pod.Containers[1].Env)
	}
	for _, container := range append(pod.InitContainers, pod.Containers...) {
		for _, environment := range container.Env {
			if strings.Contains(strings.ToLower(environment.Name), "token") || strings.Contains(strings.ToLower(environment.Name), "secret") {
				t.Fatalf("Job spec persisted secret-bearing environment variable %q", environment.Name)
			}
		}
	}
	for _, volume := range pod.Volumes {
		if volume.EmptyDir == nil || volume.EmptyDir.Medium != corev1.StorageMediumMemory {
			t.Fatalf("sandbox volume %q is not memory-backed", volume.Name)
		}
	}
	if len(pod.HostAliases) != 2 || pod.HostAliases[0].Hostnames[0] != "api.example.com" {
		t.Fatalf("declared network destination was not pinned in /etc/hosts: %v", pod.HostAliases)
	}
	policy := runner.buildNetworkPolicy(job, destinations)
	if len(policy.Spec.Egress) != 1 || len(policy.Spec.Egress[0].To) != 2 || policy.Spec.Egress[0].To[0].IPBlock.CIDR != "8.8.8.8/32" || policy.Spec.Egress[0].To[1].IPBlock.CIDR != "2606:4700:4700::1111/128" {
		t.Fatalf("unexpected pinned egress policy: %#v", policy.Spec.Egress)
	}
}

func TestKubernetesRunnerRejectsMutableImageBeforeCreatingJob(t *testing.T) {
	client := fake.NewSimpleClientset()
	runner, err := NewKubernetesRunner(client, KubernetesRunnerConfig{
		Namespace:       "sandbox",
		RunnerImage:     "runner:v1",
		SupervisorImage: "supervisor:v1",
		RuntimeClass:    "gvisor",
		ServiceAccount:  "sandbox-job",
		AppName:         "allcallall",
		Instance:        "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = runner.PrepareExecution(context.Background(), mcpplatform.ExecutionRequest{
		ExecutionID: "mcp:run-1:call-1",
		Definition:  mcpplatform.InstallationDefinition{ImageRef: "registry.example.com/tool:latest"},
	})
	if err == nil {
		t.Fatal("mutable OCI image was accepted")
	}
	jobs, listErr := client.BatchV1().Jobs("sandbox").List(context.Background(), metav1.ListOptions{})
	if listErr != nil || len(jobs.Items) != 0 {
		t.Fatalf("mutable image created Jobs: jobs=%d err=%v", len(jobs.Items), listErr)
	}
}

func TestKubernetesRunnerRejectsUnsafeNetworkAllowlistBeforeCreatingJob(t *testing.T) {
	client := fake.NewSimpleClientset()
	runner, err := NewKubernetesRunner(client, KubernetesRunnerConfig{
		Namespace:       "sandbox",
		RunnerImage:     "runner:v1",
		SupervisorImage: "supervisor:v1",
		RuntimeClass:    "gvisor",
		ServiceAccount:  "sandbox-job",
		AppName:         "allcallall",
		Instance:        "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	runner.resolver = staticIPResolver{
		"rebind.example": {{IP: net.ParseIP("100.64.0.1")}},
	}
	image := "registry.example.com/tool@sha256:" + strings.Repeat("a", 64)
	for _, allowlist := range [][]string{{"*.example.com"}, {"127.0.0.1"}, {"rebind.example"}} {
		request := receiptExecutionRequest("wrap-token")
		request.Definition.ImageRef = image
		request.Definition.NetworkAllowlist = allowlist
		if _, err := runner.PrepareExecution(context.Background(), request); err == nil {
			t.Fatalf("unsafe allowlist was accepted: %v", allowlist)
		}
	}
	jobs, listErr := client.BatchV1().Jobs("sandbox").List(context.Background(), metav1.ListOptions{})
	if listErr != nil || len(jobs.Items) != 0 {
		t.Fatalf("unsafe allowlist created Jobs: jobs=%d err=%v", len(jobs.Items), listErr)
	}
}

func TestKubernetesRunnerPinsResolvedPublicDestinations(t *testing.T) {
	runner, err := NewKubernetesRunner(fake.NewSimpleClientset(), KubernetesRunnerConfig{
		Namespace:       "sandbox",
		RunnerImage:     "runner:v1",
		SupervisorImage: "supervisor:v1",
		RuntimeClass:    "gvisor",
		ServiceAccount:  "sandbox-job",
		AppName:         "allcallall",
		Instance:        "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	runner.resolver = staticIPResolver{
		"api.example.com": {
			{IP: net.ParseIP("2606:4700:4700::1111")},
			{IP: net.ParseIP("8.8.8.8")},
			{IP: net.ParseIP("8.8.8.8")},
		},
	}
	destinations, err := runner.resolveNetworkAllowlist(context.Background(), []string{"API.EXAMPLE.COM."})
	if err != nil {
		t.Fatal(err)
	}
	if len(destinations) != 1 || destinations[0].hostname != "api.example.com" ||
		len(destinations[0].addresses) != 2 || destinations[0].addresses[0] != "2606:4700:4700::1111" || destinations[0].addresses[1] != "8.8.8.8" {
		t.Fatalf("unexpected pinned destinations: %#v", destinations)
	}
}

func TestKubernetesPreparedExecutionKeepsRequestOutOfJobSpec(t *testing.T) {
	const jobName = "allcallall-mcp-test"
	const wrapToken = "one-time-wrap-token"
	var createdJob *batchv1.Job
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sandbox-pod",
			Namespace: "sandbox",
			Labels:    map[string]string{"job-name": jobName},
		},
		Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			PodIP:             "10.0.0.8",
			ContainerStatuses: []corev1.ContainerStatus{{Name: "runner", Ready: true}},
		},
	})
	client.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		job := action.(k8stesting.CreateAction).GetObject().(*batchv1.Job).DeepCopy()
		job.Name = jobName
		createdJob = job
		return true, job, nil
	})

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var input mcpplatform.ExecutionRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Errorf("decode Runner request: %v", err)
		}
		if input.SecretWrapToken != wrapToken || input.ExecutionID != "mcp:run-1:call-1" {
			t.Errorf("unexpected in-memory Runner request: %#v", input)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"job_id":"ignored","output":{"ok":true}}`))
	}))
	defer server.Close()

	runner, err := NewKubernetesRunner(client, KubernetesRunnerConfig{
		Namespace:       "sandbox",
		RunnerImage:     "runner:v1",
		SupervisorImage: "supervisor:v1",
		RuntimeClass:    "gvisor",
		ServiceAccount:  "sandbox-job",
		AppName:         "allcallall",
		Instance:        "test",
		PollInterval:    time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	runner.endpointForPod = func(string) string { return server.URL }
	request := receiptExecutionRequest(wrapToken)
	request.ExecutionID = "mcp:run-1:call-1"
	prepared, err := runner.PrepareExecution(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.JobID() != jobName {
		t.Fatalf("Job ID=%q", prepared.JobID())
	}
	policies, err := client.NetworkingV1().NetworkPolicies("sandbox").List(context.Background(), metav1.ListOptions{})
	if err != nil || len(policies.Items) != 1 || len(policies.Items[0].Spec.Egress) != 0 {
		t.Fatalf("zero-egress NetworkPolicy was not created: policies=%d err=%v", len(policies.Items), err)
	}
	encodedJob, err := json.Marshal(createdJob)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedJob), wrapToken) {
		t.Fatal("one-time wrapping token was persisted in the Kubernetes Job")
	}
	result, err := prepared.Execute(context.Background())
	if err != nil || result.Output["ok"] != true || result.JobID != jobName {
		t.Fatalf("execute prepared Job: result=%#v err=%v", result, err)
	}
	if err := prepared.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	policies, err = client.NetworkingV1().NetworkPolicies("sandbox").List(context.Background(), metav1.ListOptions{})
	if err != nil || len(policies.Items) != 0 {
		t.Fatalf("NetworkPolicy was not deleted: policies=%d err=%v", len(policies.Items), err)
	}
}

func assertLockedContainer(t *testing.T, container corev1.Container) {
	t.Helper()
	security := container.SecurityContext
	if security == nil || security.AllowPrivilegeEscalation == nil || *security.AllowPrivilegeEscalation ||
		security.ReadOnlyRootFilesystem == nil || !*security.ReadOnlyRootFilesystem ||
		security.RunAsNonRoot == nil || !*security.RunAsNonRoot || security.Capabilities == nil {
		t.Fatalf("container %q is not locked down: %#v", container.Name, security)
	}
	if len(security.Capabilities.Drop) != 1 || security.Capabilities.Drop[0] != "ALL" {
		t.Fatalf("container %q capabilities=%v", container.Name, security.Capabilities.Drop)
	}
}
