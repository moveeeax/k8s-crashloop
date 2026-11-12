package analyze

import (
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Causes of a container's last termination, as reported by Restart.Cause.
const (
	CauseOOMKilled    = "OOMKilled"     // terminated reason OOMKilled
	CauseLiveness     = "LivenessProbe" // kubelet killed it after a failed liveness probe
	CauseStartupProbe = "StartupProbe"  // kubelet killed it after a failed startup probe
	CauseError        = "Error"         // non-zero exit from the application
	CauseCompleted    = "Completed"     // exit code 0
	CauseUnknown      = "Unknown"       // no last termination state recorded
)

// KillingReason is the event reason the kubelet records when it kills a container.
const KillingReason = "Killing"

// probeSlack absorbs clock skew between the event timestamp and the container's
// recorded start/finish times.
const probeSlack = 5 * time.Second

// Restart is one container that has restarted at least once.
type Restart struct {
	Namespace    string     `json:"namespace"`
	Pod          string     `json:"pod"`
	Container    string     `json:"container"`
	Init         bool       `json:"init,omitempty"`
	Owner        string     `json:"owner"`
	Node         string     `json:"node,omitempty"`
	RestartCount int32      `json:"restart_count"`
	Cause        string     `json:"cause"`
	Reason       string     `json:"reason,omitempty"`
	ExitCode     int32      `json:"exit_code"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	Waiting      string     `json:"waiting,omitempty"`
}

// Sort orders accepted by SortRestarts and SortOwners.
const (
	SortByCount  = "count"
	SortByRecent = "recent"
)

// Restarts returns every container (init containers included) with a non-zero
// restart count. When since > 0, containers whose last termination finished
// before now-since are dropped, so a loop that already settled falls off the
// list. Restart counts are the kubelet's lifetime totals for the pod; the API
// does not expose per-window counts.
//
// events are the pods' "Killing" events; they are only used to tell probe kills
// apart from application exits and may be nil.
func Restarts(pods []corev1.Pod, events []corev1.Event, since time.Duration, now time.Time) []Restart {
	probeKills := indexProbeKills(events)
	var out []Restart
	for _, pod := range pods {
		add := func(cs corev1.ContainerStatus, init bool) {
			if cs.RestartCount == 0 {
				return
			}
			r := Restart{
				Namespace:    pod.Namespace,
				Pod:          pod.Name,
				Container:    cs.Name,
				Init:         init,
				Owner:        Owner(pod),
				Node:         pod.Spec.NodeName,
				RestartCount: cs.RestartCount,
				Cause:        CauseUnknown,
			}
			if cs.State.Waiting != nil {
				r.Waiting = cs.State.Waiting.Reason
			}
			if term := cs.LastTerminationState.Terminated; term != nil {
				r.Reason = term.Reason
				r.ExitCode = term.ExitCode
				if !term.FinishedAt.IsZero() {
					t := term.FinishedAt.Time
					r.FinishedAt = &t
				}
				r.Cause = classify(term, probeKills[podKey{pod.Namespace, pod.Name, cs.Name}])
			}
			if since > 0 && (r.FinishedAt == nil || r.FinishedAt.Before(now.Add(-since))) {
				return
			}
			out = append(out, r)
		}
		for _, cs := range pod.Status.InitContainerStatuses {
			add(cs, true)
		}
		for _, cs := range pod.Status.ContainerStatuses {
			add(cs, false)
		}
	}
	SortRestarts(out, SortByCount)
	return out
}

// SortRestarts orders restarts in place: by restart count descending ("count")
// or by last finish time, newest first ("recent"). Ties fall back to
// namespace/pod/container so output is stable.
func SortRestarts(rs []Restart, by string) {
	sort.SliceStable(rs, func(i, j int) bool {
		a, b := rs[i], rs[j]
		if by == SortByRecent {
			if c := cmpTime(a.FinishedAt, b.FinishedAt); c != 0 {
				return c > 0
			}
		}
		if a.RestartCount != b.RestartCount {
			return a.RestartCount > b.RestartCount
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		if a.Pod != b.Pod {
			return a.Pod < b.Pod
		}
		return a.Container < b.Container
	})
}

// ValidSort reports whether by is a supported sort order.
func ValidSort(by string) error {
	switch by {
	case SortByCount, SortByRecent:
		return nil
	}
	return fmt.Errorf("invalid sort %q (want %s or %s)", by, SortByCount, SortByRecent)
}

// Owner names the workload a pod belongs to as "Kind/name". Pods created by a
// Deployment's ReplicaSet roll up to the Deployment: the ReplicaSet name is the
// Deployment name plus "-<pod-template-hash>", so no extra API call is needed.
// Pods without a controller are reported as "Pod/<name>".
func Owner(pod corev1.Pod) string {
	ref := metav1.GetControllerOf(&pod)
	if ref == nil {
		return "Pod/" + pod.Name
	}
	if ref.Kind == "ReplicaSet" {
		if h := pod.Labels["pod-template-hash"]; h != "" && strings.HasSuffix(ref.Name, "-"+h) {
			return "Deployment/" + strings.TrimSuffix(ref.Name, "-"+h)
		}
	}
	return ref.Kind + "/" + ref.Name
}

// OwnerRestarts is the restart total for one workload.
type OwnerRestarts struct {
	Namespace  string         `json:"namespace"`
	Owner      string         `json:"owner"`
	Pods       int            `json:"pods"`
	Containers int            `json:"containers"`
	Restarts   int32          `json:"restarts"`
	Causes     map[string]int `json:"causes"`
	FinishedAt *time.Time     `json:"last_finished_at,omitempty"`
}

// ByOwner sums restarts per namespace/owner. Causes counts containers by the
// cause of their last termination; FinishedAt is the most recent one.
func ByOwner(rs []Restart) []OwnerRestarts {
	type key struct{ ns, owner string }
	idx := map[key]*OwnerRestarts{}
	pods := map[key]map[string]bool{}
	var order []key
	for _, r := range rs {
		k := key{r.Namespace, r.Owner}
		o, ok := idx[k]
		if !ok {
			o = &OwnerRestarts{Namespace: r.Namespace, Owner: r.Owner, Causes: map[string]int{}}
			idx[k] = o
			pods[k] = map[string]bool{}
			order = append(order, k)
		}
		pods[k][r.Pod] = true
		o.Containers++
		o.Restarts += r.RestartCount
		o.Causes[r.Cause]++
		if cmpTime(r.FinishedAt, o.FinishedAt) > 0 {
			o.FinishedAt = r.FinishedAt
		}
	}
	out := make([]OwnerRestarts, 0, len(order))
	for _, k := range order {
		o := idx[k]
		o.Pods = len(pods[k])
		out = append(out, *o)
	}
	SortOwners(out, SortByCount)
	return out
}

// SortOwners orders owners in place, with the same keys as SortRestarts.
func SortOwners(owners []OwnerRestarts, by string) {
	sort.SliceStable(owners, func(i, j int) bool {
		a, b := owners[i], owners[j]
		if by == SortByRecent {
			if c := cmpTime(a.FinishedAt, b.FinishedAt); c != 0 {
				return c > 0
			}
		}
		if a.Restarts != b.Restarts {
			return a.Restarts > b.Restarts
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		return a.Owner < b.Owner
	})
}

// classify picks the cause of a termination. A probe kill is recognised by a
// kubelet "Killing" event for the container that falls inside the terminated
// run; the runtime itself only reports exit 137/143 with reason "Error" there.
func classify(term *corev1.ContainerStateTerminated, kills []probeKill) string {
	if term.Reason == "OOMKilled" {
		return CauseOOMKilled
	}
	for _, k := range kills {
		if !term.StartedAt.IsZero() && k.at.Before(term.StartedAt.Add(-probeSlack)) {
			continue
		}
		if !term.FinishedAt.IsZero() && k.at.After(term.FinishedAt.Add(probeSlack)) {
			continue
		}
		return k.cause
	}
	if term.ExitCode == 0 {
		return CauseCompleted
	}
	return CauseError
}

type podKey struct{ ns, pod, container string }

type probeKill struct {
	at    time.Time
	cause string
}

// indexProbeKills keeps the kubelet "Killing" events whose message says a
// liveness or startup probe failed. The kubelet writes
// "Container <name> failed liveness probe, will be restarted" (and the same for
// startup) with the container's field path as the involved object.
func indexProbeKills(events []corev1.Event) map[podKey][]probeKill {
	out := map[podKey][]probeKill{}
	for _, e := range events {
		if e.Reason != KillingReason || e.InvolvedObject.Kind != "Pod" {
			continue
		}
		container := fieldPathContainer(e.InvolvedObject.FieldPath)
		if container == "" {
			continue
		}
		var cause string
		switch {
		case strings.Contains(e.Message, "failed liveness probe"):
			cause = CauseLiveness
		case strings.Contains(e.Message, "failed startup probe"):
			cause = CauseStartupProbe
		default:
			continue
		}
		at := eventTime(e)
		if at.IsZero() {
			continue
		}
		k := podKey{e.InvolvedObject.Namespace, e.InvolvedObject.Name, container}
		out[k] = append(out[k], probeKill{at: at, cause: cause})
	}
	return out
}

// fieldPathContainer extracts the container name from "spec.containers{name}"
// or "spec.initContainers{name}".
func fieldPathContainer(fp string) string {
	for _, prefix := range []string{"spec.containers{", "spec.initContainers{"} {
		if strings.HasPrefix(fp, prefix) && strings.HasSuffix(fp, "}") {
			return strings.TrimSuffix(strings.TrimPrefix(fp, prefix), "}")
		}
	}
	return ""
}

// eventTime is the latest time the event was seen. Repeated events are
// aggregated, so LastTimestamp is the most recent occurrence.
func eventTime(e corev1.Event) time.Time {
	switch {
	case e.Series != nil && !e.Series.LastObservedTime.IsZero():
		return e.Series.LastObservedTime.Time
	case !e.LastTimestamp.IsZero():
		return e.LastTimestamp.Time
	case !e.EventTime.IsZero():
		return e.EventTime.Time
	}
	return e.FirstTimestamp.Time
}

// cmpTime compares two optional times; nil sorts before any time.
func cmpTime(a, b *time.Time) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	}
	return a.Compare(*b)
}
