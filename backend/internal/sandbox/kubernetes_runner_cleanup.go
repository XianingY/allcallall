package sandbox

import (
	"context"
	"errors"
	"fmt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
	"time"
)

func (r *KubernetesRunner) deleteJob(jobName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := r.deleteJobWithContext(ctx, jobName); err != nil {
		log.Warn().Err(err).Str("job", jobName).Msg("failed to delete sandbox job")
	}
}

func (r *KubernetesRunner) deleteResources(jobName, policyName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := r.deleteResourcesWithContext(ctx, jobName, policyName); err != nil {
		log.Warn().Err(err).Str("job", jobName).Str("policy", policyName).Msg("failed to delete sandbox resources")
	}
}

func (r *KubernetesRunner) deleteResourcesWithContext(ctx context.Context, jobName, policyName string) error {
	var policyErr error
	if strings.TrimSpace(policyName) != "" {
		policyErr = r.client.NetworkingV1().NetworkPolicies(r.config.Namespace).Delete(ctx, policyName, metav1.DeleteOptions{})
		if apierrors.IsNotFound(policyErr) {
			policyErr = nil
		} else if policyErr != nil {
			policyErr = fmt.Errorf("delete isolated sandbox NetworkPolicy: %w", policyErr)
		}
	}
	return errors.Join(policyErr, r.deleteJobWithContext(ctx, jobName))
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
