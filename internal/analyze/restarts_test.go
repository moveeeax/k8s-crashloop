package analyze

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var now = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

func terminated(name string, restarts int32, reason string, exit int32, start, finish time.Time) corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name:         name,
		RestartCount: restarts,
		LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
			Reason: reason, ExitCode: exit,
			StartedAt: metav1.NewTime(start), FinishedAt: metav1.NewTime(finish),
		}},
	}
}

func killing(pod, container, msg string, at time.Time) corev1.Event {
	return corev1.Event{
		Reason:        KillingReason,
		Message:       msg,
		LastTimestamp: metav1.NewTime(at),
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod", Namespace: "default", Name: pod, FieldPath: "spec.containers{" + container + "}",
		},
	}
}

func TestRestartsClassifiesCauses(t *testing.T) {
	start, fin := now.Add(-10*time.Minute), now.Add(-5*time.Minute)
	pods := []corev1.Pod{
		pod("default", "oom", terminated("app", 5, "OOMKilled", 137, start, fin)),
		pod("default", "probe", terminated("app", 4, "Error", 137, start, fin)),
		pod("default", "startup", terminated("app", 3, "Error", 143, start, fin)),
		pod("default", "crash", terminated("app", 2, "Error", 1, start, fin)),
		pod("default", "done", terminated("app", 1, "Completed", 0, start, fin)),
		pod("default", "stale", terminated("app", 1, "Error", 137, start, fin)),
	}
	events := []corev1.Event{
		killing("probe", "app", "Container app failed liveness probe, will be restarted", fin.Add(-30*time.Second)),
		killing("startup", "app", "Container app failed startup probe, will be restarted", fin.Add(-time.Second)),
		// A probe kill from an earlier run must not be attributed to the last one.
		killing("stale", "app", "Container app failed liveness probe, will be restarted", start.Add(-time.Hour)),
		// Killing for other reasons is not a probe kill.
		killing("crash", "app", "Stopping container app", fin),
	}
	want := map[string]string{
		"oom": CauseOOMKilled, "probe": CauseLiveness, "startup": CauseStartupProbe,
		"crash": CauseError, "done": CauseCompleted, "stale": CauseError,
	}
	rs := Restarts(pods, events, 0, now)
	if len(rs) != len(want) {
		t.Fatalf("want %d restarts, got %d", len(want), len(rs))
	}
	for _, r := range rs {
		if r.Cause != want[r.Pod] {
			t.Errorf("%s: cause = %s, want %s", r.Pod, r.Cause, want[r.Pod])
		}
	}
	if rs[0].Pod != "oom" || rs[0].FinishedAt == nil || !rs[0].FinishedAt.Equal(fin) {
		t.Errorf("first = %+v", rs[0])
	}
}

func TestRestartsSkipsZeroAndIncludesInit(t *testing.T) {
	p := pod("default", "web", corev1.ContainerStatus{Name: "app"})
	p.Status.InitContainerStatuses = []corev1.ContainerStatus{
		terminated("migrate", 3, "Error", 1, now.Add(-2*time.Minute), now.Add(-time.Minute)),
	}
	rs := Restarts([]corev1.Pod{p}, nil, 0, now)
	if len(rs) != 1 || rs[0].Container != "migrate" || !rs[0].Init {
		t.Fatalf("want only the init container, got %+v", rs)
	}
}

func TestRestartsSinceWindow(t *testing.T) {
	pods := []corev1.Pod{
		pod("default", "recent", terminated("app", 2, "Error", 1, now.Add(-2*time.Hour), now.Add(-time.Hour))),
		pod("default", "settled", terminated("app", 50, "Error", 1, now.Add(-49*time.Hour), now.Add(-48*time.Hour))),
		pod("default", "nostate", corev1.ContainerStatus{Name: "app", RestartCount: 1}),
	}
	rs := Restarts(pods, nil, 24*time.Hour, now)
	if len(rs) != 1 || rs[0].Pod != "recent" {
		t.Fatalf("want only recent, got %+v", rs)
	}
	if all := Restarts(pods, nil, 0, now); len(all) != 3 {
		t.Fatalf("without --since want 3, got %d", len(all))
	}
	if all := Restarts(pods, nil, 0, now); all[2].Cause != CauseUnknown {
		t.Errorf("no last state should be Unknown, got %s", all[2].Cause)
	}
}

func TestSortRestartsRecent(t *testing.T) {
	older, newer := now.Add(-time.Hour), now.Add(-time.Minute)
	rs := []Restart{
		{Pod: "many-old", RestartCount: 30, FinishedAt: &older},
		{Pod: "none", RestartCount: 50},
		{Pod: "few-new", RestartCount: 2, FinishedAt: &newer},
	}
	SortRestarts(rs, SortByRecent)
	if rs[0].Pod != "few-new" || rs[1].Pod != "many-old" || rs[2].Pod != "none" {
		t.Errorf("wrong order: %s %s %s", rs[0].Pod, rs[1].Pod, rs[2].Pod)
	}
	SortRestarts(rs, SortByCount)
	if rs[0].Pod != "none" {
		t.Errorf("count order should put 50 first, got %s", rs[0].Pod)
	}
	if ValidSort("name") == nil || ValidSort(SortByRecent) != nil {
		t.Error("ValidSort")
	}
}

func TestOwner(t *testing.T) {
	ctrl := true
	withOwner := func(kind, name string, labels map[string]string) corev1.Pod {
		p := pod("default", "p", nil...)
		p.Labels = labels
		p.OwnerReferences = []metav1.OwnerReference{{Kind: kind, Name: name, Controller: &ctrl}}
		return p
	}
	cases := []struct {
		pod  corev1.Pod
		want string
	}{
		{withOwner("ReplicaSet", "api-5d8f", map[string]string{"pod-template-hash": "5d8f"}), "Deployment/api"},
		{withOwner("ReplicaSet", "bare-rs", nil), "ReplicaSet/bare-rs"},
		{withOwner("StatefulSet", "db", nil), "StatefulSet/db"},
		{withOwner("DaemonSet", "agent", nil), "DaemonSet/agent"},
		{pod("default", "solo"), "Pod/solo"},
	}
	for _, c := range cases {
		if got := Owner(c.pod); got != c.want {
			t.Errorf("Owner = %s, want %s", got, c.want)
		}
	}
}

func TestByOwner(t *testing.T) {
	t1, t2 := now.Add(-time.Hour), now.Add(-time.Minute)
	rs := []Restart{
		{Namespace: "a", Pod: "api-1", Container: "app", Owner: "Deployment/api", RestartCount: 3, Cause: CauseError, FinishedAt: &t1},
		{Namespace: "a", Pod: "api-1", Container: "sidecar", Owner: "Deployment/api", RestartCount: 1, Cause: CauseError},
		{Namespace: "a", Pod: "api-2", Container: "app", Owner: "Deployment/api", RestartCount: 4, Cause: CauseOOMKilled, FinishedAt: &t2},
		{Namespace: "a", Pod: "db-0", Container: "db", Owner: "StatefulSet/db", RestartCount: 10, Cause: CauseLiveness},
	}
	os := ByOwner(rs)
	if len(os) != 2 || os[0].Owner != "StatefulSet/db" {
		t.Fatalf("unexpected: %+v", os)
	}
	api := os[1]
	if api.Pods != 2 || api.Containers != 3 || api.Restarts != 8 || api.Causes[CauseError] != 2 || api.Causes[CauseOOMKilled] != 1 {
		t.Errorf("api totals: %+v", api)
	}
	if api.FinishedAt == nil || !api.FinishedAt.Equal(t2) {
		t.Errorf("last finish = %v", api.FinishedAt)
	}
	SortOwners(os, SortByRecent)
	if os[0].Owner != "Deployment/api" {
		t.Errorf("recent order should put api first, got %s", os[0].Owner)
	}
}
