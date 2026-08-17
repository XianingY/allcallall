package sandbox

import (
	"context"
	"errors"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	"strings"
	"time"
)

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
