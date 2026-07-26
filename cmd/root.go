// Package cmd wires the k8s-crashloop CLI.
package cmd

import (
	"context"
	"io"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"

	"github.com/moveeeax/k8s-crashloop/internal/analyze"
	"github.com/moveeeax/k8s-crashloop/internal/kube"
	"github.com/moveeeax/k8s-crashloop/internal/report"
)

// providers is overridable in tests.
var providers = func(kubeconfig string) (kube.Source, kube.LogFetcher, error) {
	c, err := kube.New(kubeconfig)
	return c, c, err
}

func newRootCmd() *cobra.Command {
	var (
		namespace     string
		allNamespaces bool
		tail          int64
		asJSON        bool
		kubeconfig    string
		noColor       bool
	)

	root := &cobra.Command{
		Use:           "k8s-crashloop",
		Short:         "See every CrashLoopBackOff pod and why it's dying",
		Long:          "k8s-crashloop lists CrashLoopBackOff containers across a cluster with their\nrestart count, exit code, last state, and the previous container's tail logs.",
		Example:       "  k8s-crashloop --all-namespaces --tail 20\n  k8s-crashloop -n prod --json",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var namespaces []string
			if !allNamespaces && namespace != "" {
				namespaces = []string{namespace}
			}

			src, logs, err := providers(kubeconfig)
			if err != nil {
				return err
			}
			ctx := context.Background()
			pods, err := src.Pods(ctx, namespaces)
			if err != nil {
				return err
			}
			findings := analyze.Find(pods)

			if tail > 0 {
				for i := range findings {
					if l, err := logs.PreviousLogs(ctx, findings[i].Namespace, findings[i].Pod, findings[i].Container, tail); err == nil {
						findings[i].Logs = l
					}
				}
			}

			if asJSON {
				return report.JSON(cmd.OutOrStdout(), findings)
			}
			report.Table(cmd.OutOrStdout(), findings, !noColor)
			return nil
		},
	}

	root.Flags().StringVarP(&namespace, "namespace", "n", "", "namespace to scan")
	root.Flags().BoolVarP(&allNamespaces, "all-namespaces", "A", false, "scan all namespaces")
	root.Flags().Int64Var(&tail, "tail", 20, "lines of previous-container logs to fetch (0 to skip)")
	root.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
	root.Flags().BoolVar(&noColor, "no-color", false, "disable colored output")
	root.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig (default: standard loading rules)")
	return root
}

// Execute runs the root command.
func Execute() error {
	return newRootCmd().Execute()
}

// helper exported for tests to build fixture pods succinctly is unnecessary;
// tests import corev1 directly.
var _ = corev1.Pod{}

// RunWith lets tests drive the command with injected streams.
func RunWith(args []string, out io.Writer) error {
	c := newRootCmd()
	c.SetOut(out)
	c.SetErr(out)
	c.SetArgs(args)
	return c.Execute()
}
