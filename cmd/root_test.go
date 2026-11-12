package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/moveeeax/k8s-crashloop/internal/analyze"
	"github.com/moveeeax/k8s-crashloop/internal/kube"
)

type fakeKube struct {
	pods      []corev1.Pod
	logs      string
	events    []corev1.Event
	eventsErr error
}

func (f fakeKube) Pods(context.Context, []string) ([]corev1.Pod, error) { return f.pods, nil }
func (f fakeKube) PreviousLogs(context.Context, string, string, string, int64) (string, error) {
	return f.logs, nil
}
func (f fakeKube) KillingEvents(context.Context, []string) ([]corev1.Event, error) {
	return f.events, f.eventsErr
}

func withKube(t *testing.T, f fakeKube) {
	t.Helper()
	orig := providers
	providers = func(string) (kube.Cluster, error) { return f, nil }
	t.Cleanup(func() { providers = orig })
}

func crashPod() corev1.Pod {
	p := corev1.Pod{}
	p.Namespace = "prod"
	p.Name = "api"
	p.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name:         "api",
		RestartCount: 8,
		State:        corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: analyze.CrashLoopReason}},
		LastTerminationState: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{ExitCode: 1},
		},
	}}
	return p
}

func TestRunTable(t *testing.T) {
	withKube(t, fakeKube{pods: []corev1.Pod{crashPod()}, logs: "boom"})
	var out bytes.Buffer
	if err := RunWith([]string{"-A", "--no-color"}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "api") || !strings.Contains(s, "boom") {
		t.Errorf("unexpected output:\n%s", s)
	}
}

func TestRunJSON(t *testing.T) {
	withKube(t, fakeKube{pods: []corev1.Pod{crashPod()}, logs: "boom"})
	var out bytes.Buffer
	if err := RunWith([]string{"-A", "--json", "--tail", "5"}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	var fs []analyze.Finding
	if err := json.Unmarshal(out.Bytes(), &fs); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if len(fs) != 1 || fs[0].Logs != "boom" {
		t.Errorf("unexpected findings: %+v", fs)
	}
}

func TestRunNoFindings(t *testing.T) {
	withKube(t, fakeKube{})
	var out bytes.Buffer
	if err := RunWith([]string{"-A"}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "No CrashLoopBackOff") {
		t.Errorf("unexpected: %s", out.String())
	}
}
