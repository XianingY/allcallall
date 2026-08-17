package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/allcallall/backend/internal/mcpplatform"
	"io"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

func (r *KubernetesRunner) Validate(ctx context.Context, request mcpplatform.ValidationRequest) (mcpplatform.ValidationResult, error) {
	job, policy, endpoint, err := r.preparePod(ctx, request.Definition, "validate", "")
	if err != nil {
		return mcpplatform.ValidationResult{}, err
	}
	defer r.deleteResources(job, policy)
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
	job, policy, endpoint, err := r.preparePod(ctx, request.Definition, "execute", request.ExecutionID)
	if err != nil {
		return nil, err
	}
	return &kubernetesPreparedExecution{
		runner:        r,
		jobName:       job,
		networkPolicy: policy,
		endpoint:      endpoint,
		request:       request,
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
	return p.runner.deleteResourcesWithContext(ctx, p.jobName, p.networkPolicy)
}

func (r *KubernetesRunner) preparePod(ctx context.Context, definition mcpplatform.InstallationDefinition, operation, executionID string) (string, string, string, error) {
	if err := validateDigestPinned(definition.ImageRef); err != nil {
		return "", "", "", err
	}
	destinations, err := r.resolveNetworkAllowlist(ctx, definition.NetworkAllowlist)
	if err != nil {
		return "", "", "", err
	}
	job := r.buildJob(definition.ImageRef, operation, executionID, destinations)
	created, err := r.client.BatchV1().Jobs(r.config.Namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return "", "", "", fmt.Errorf("create isolated sandbox Job: %w", err)
	}
	jobName := created.Name
	policy := r.buildNetworkPolicy(created, destinations)
	createdPolicy, err := r.client.NetworkingV1().NetworkPolicies(r.config.Namespace).Create(ctx, policy, metav1.CreateOptions{})
	if err != nil {
		r.deleteJob(jobName)
		return "", "", "", fmt.Errorf("create isolated sandbox NetworkPolicy: %w", err)
	}
	policyName := createdPolicy.Name
	waitCtx, cancel := context.WithTimeout(ctx, r.config.StartupTimeout)
	defer cancel()
	podIP, err := r.waitForRunner(waitCtx, jobName)
	if err != nil {
		r.deleteResources(jobName, policyName)
		return "", "", "", err
	}
	return jobName, policyName, r.endpointForPod(podIP), nil
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
