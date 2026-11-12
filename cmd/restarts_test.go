package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/moveeeax/k8s-crashloop/internal/analyze"
)

var fixedNow = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

func withNow(t *testing.T) {
	t.Helper()
	orig := now
	now = func() time.Time { return fixedNow }
	t.Cleanup(func() { now = orig })
}

func restartedPod(name, rs, hash string, restarts int32, reason string, exit int32, finishedAgo time.Duration) corev1.Pod {
	p := corev1.Pod{}
	p.Namespace = "prod"
	p.Name = name
	p.Labels = map[string]string{"pod-template-hash": hash}
	ctrl := true
	p.OwnerReferences = []metav1.OwnerReference{{Kind: "ReplicaSet", Name: rs, Controller: &ctrl}}
	fin := fixedNow.Add(-finishedAgo)
	p.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name:         "app",
		RestartCount: restarts,
		State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
		LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
			Reason: reason, ExitCode: exit,
			StartedAt:  metav1.NewTime(fin.Add(-time.Minute)),
			FinishedAt: metav1.NewTime(fin),
		}},
	}}
	return p
}

func restartFixture() fakeKube {
	livenessAt := fixedNow.Add(-2*time.Hour - 10*time.Second)
	return fakeKube{
		pods: []corev1.Pod{
			restartedPod("api-1", "api-5d8f", "5d8f", 9, "OOMKilled", 137, 30*time.Minute),
			restartedPod("api-2", "api-5d8f", "5d8f", 4, "Error", 1, 72*time.Hour),
			restartedPod("web-1", "web-77aa", "77aa", 6, "Error", 137, 2*time.Hour),
		},
		events: []corev1.Event{{
			Reason:        analyze.KillingReason,
			Message:       "Container app failed liveness probe, will be restarted",
			LastTimestamp: metav1.NewTime(livenessAt),
			InvolvedObject: corev1.ObjectReference{
				Kind: "Pod", Namespace: "prod", Name: "web-1", FieldPath: "spec.containers{app}",
			},
		}},
	}
}

func TestRestartsTable(t *testing.T) {
	withNow(t)
	withKube(t, restartFixture())
	var out bytes.Buffer
	if err := RunWith([]string{"restarts", "-A", "--no-color"}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	s := out.String()
	for _, sub := range []string{"CAUSE", "OOMKilled", "LivenessProbe", "Error", "30m", "3 restarted container(s)"} {
		if !strings.Contains(s, sub) {
			t.Errorf("missing %q:\n%s", sub, s)
		}
	}
	if strings.Contains(s, "OWNER") {
		t.Errorf("owner column belongs to -o wide:\n%s", s)
	}
	if i, j := strings.Index(s, "api-1"), strings.Index(s, "web-1"); i > j {
		t.Errorf("api-1 (9 restarts) should come before web-1 (6):\n%s", s)
	}
}

func TestRestartsSinceAndTop(t *testing.T) {
	withNow(t)
	withKube(t, restartFixture())
	var out bytes.Buffer
	if err := RunWith([]string{"restarts", "-A", "--since", "24h", "--sort-by", "recent", "--top", "1", "-o", "json"}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	var rs []analyze.Restart
	if err := json.Unmarshal(out.Bytes(), &rs); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if len(rs) != 1 || rs[0].Pod != "api-1" {
		t.Errorf("want only api-1 (most recent within 24h), got %+v", rs)
	}
}

func TestRestartsByOwnerWide(t *testing.T) {
	withNow(t)
	withKube(t, restartFixture())
	var out bytes.Buffer
	if err := RunWith([]string{"restarts", "-A", "--by-owner", "-o", "wide", "--no-color"}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	s := out.String()
	for _, sub := range []string{"Deployment/api", "Deployment/web", "Error:1,OOMKilled:1", "2026-11-05T11:30:00Z", "2 workload(s)"} {
		if !strings.Contains(s, sub) {
			t.Errorf("missing %q:\n%s", sub, s)
		}
	}
}

func TestRestartsEventsErrorFallsBack(t *testing.T) {
	withNow(t)
	f := restartFixture()
	f.eventsErr = errors.New("events is forbidden")
	withKube(t, f)
	var out bytes.Buffer
	if err := RunWith([]string{"restarts", "-A", "-o", "json"}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "warning: cannot list events") {
		t.Errorf("expected warning:\n%s", s)
	}
	if strings.Contains(s, analyze.CauseLiveness) {
		t.Errorf("no events, so no liveness cause expected:\n%s", s)
	}
}

func TestRestartsRejectsBadFlags(t *testing.T) {
	withKube(t, fakeKube{})
	for _, args := range [][]string{
		{"restarts", "--sort-by", "name"},
		{"restarts", "-o", "yaml"},
		{"restarts", "--top", "-1"},
		{"restarts", "--since", "-1h"},
	} {
		if err := RunWith(args, &bytes.Buffer{}); err == nil {
			t.Errorf("%v: want error", args)
		}
	}
}
