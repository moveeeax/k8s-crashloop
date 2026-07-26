package analyze

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func crashing(name string, restarts, exit int32) corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name:         name,
		RestartCount: restarts,
		State: corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{Reason: CrashLoopReason},
		},
		LastTerminationState: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{ExitCode: exit, Reason: "Error"},
		},
	}
}

func pod(ns, name string, statuses ...corev1.ContainerStatus) corev1.Pod {
	p := corev1.Pod{}
	p.Namespace = ns
	p.Name = name
	p.Status.ContainerStatuses = statuses
	return p
}

func TestFindIdentifiesCrashLoop(t *testing.T) {
	pods := []corev1.Pod{
		pod("default", "web", crashing("app", 7, 1)),
		pod("default", "ok", corev1.ContainerStatus{
			Name: "app", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
		}),
	}
	fs := Find(pods)
	if len(fs) != 1 {
		t.Fatalf("want 1 finding, got %d", len(fs))
	}
	f := fs[0]
	if f.Container != "app" || f.RestartCount != 7 || f.ExitCode != 1 {
		t.Errorf("unexpected finding: %+v", f)
	}
	if f.LastState != "Terminated:Error" {
		t.Errorf("last state = %q", f.LastState)
	}
}

func TestFindSortsByRestartsDesc(t *testing.T) {
	pods := []corev1.Pod{
		pod("a", "low", crashing("c", 2, 1)),
		pod("a", "high", crashing("c", 20, 137)),
		pod("a", "mid", crashing("c", 5, 1)),
	}
	fs := Find(pods)
	if len(fs) != 3 {
		t.Fatalf("want 3, got %d", len(fs))
	}
	if fs[0].Pod != "high" || fs[2].Pod != "low" {
		t.Errorf("wrong order: %v", []string{fs[0].Pod, fs[1].Pod, fs[2].Pod})
	}
}

func TestFindIgnoresNonCrashLoopWaiting(t *testing.T) {
	p := pod("default", "pending", corev1.ContainerStatus{
		Name:  "app",
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}},
	})
	if fs := Find([]corev1.Pod{p}); len(fs) != 0 {
		t.Fatalf("ContainerCreating should not be flagged: %+v", fs)
	}
}

func TestFindMultipleContainers(t *testing.T) {
	p := pod("default", "multi", crashing("a", 3, 1), crashing("b", 9, 2))
	fs := Find([]corev1.Pod{p})
	if len(fs) != 2 {
		t.Fatalf("want 2 findings, got %d", len(fs))
	}
	if fs[0].Container != "b" {
		t.Errorf("higher restarts should sort first, got %s", fs[0].Container)
	}
}
