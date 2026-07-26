// Package kube provides the client-go backed pod source and log fetcher.
package kube

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Source lists pods across the requested namespaces. It is the seam the CLI
// depends on, so the analysis runs against fixtures in tests.
type Source interface {
	Pods(ctx context.Context, namespaces []string) ([]corev1.Pod, error)
}

// LogFetcher returns the previous container's tail logs (best effort).
type LogFetcher interface {
	PreviousLogs(ctx context.Context, namespace, pod, container string, tail int64) (string, error)
}

// Client is the client-go implementation of Source and LogFetcher.
type Client struct {
	cs *kubernetes.Clientset
}

// New builds a Client from a kubeconfig path (empty uses the default loading rules).
func New(kubeconfig string) (*Client, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		rules.ExplicitPath = kubeconfig
	}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{cs: cs}, nil
}

// Pods lists pods in the given namespaces (empty slice means all namespaces).
func (c *Client) Pods(ctx context.Context, namespaces []string) ([]corev1.Pod, error) {
	if len(namespaces) == 0 {
		namespaces = []string{""} // "" = all namespaces
	}
	var out []corev1.Pod
	for _, ns := range namespaces {
		list, err := c.cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		out = append(out, list.Items...)
	}
	return out, nil
}

// PreviousLogs fetches the last `tail` lines from the previous container instance.
func (c *Client) PreviousLogs(ctx context.Context, namespace, pod, container string, tail int64) (string, error) {
	req := c.cs.CoreV1().Pods(namespace).GetLogs(pod, &corev1.PodLogOptions{
		Container: container, Previous: true, TailLines: &tail,
	})
	b, err := req.DoRaw(ctx)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
