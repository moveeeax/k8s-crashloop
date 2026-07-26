// Package analyze finds CrashLoopBackOff containers in a set of pods.
package analyze

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
)

// Finding is one crashlooping container.
type Finding struct {
	Namespace    string `json:"namespace"`
	Pod          string `json:"pod"`
	Container    string `json:"container"`
	RestartCount int32  `json:"restart_count"`
	Reason       string `json:"reason"`
	ExitCode     int32  `json:"exit_code"`
	LastState    string `json:"last_state"`
	Logs         string `json:"logs,omitempty"`
}

// CrashLoopReason is the waiting reason Kubernetes sets on a backing-off container.
const CrashLoopReason = "CrashLoopBackOff"

// Find returns the containers currently in CrashLoopBackOff, sorted by restart
// count descending (then namespace/pod/container for stability).
func Find(pods []corev1.Pod) []Finding {
	var out []Finding
	for _, pod := range pods {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting == nil || cs.State.Waiting.Reason != CrashLoopReason {
				continue
			}
			f := Finding{
				Namespace:    pod.Namespace,
				Pod:          pod.Name,
				Container:    cs.Name,
				RestartCount: cs.RestartCount,
				Reason:       cs.State.Waiting.Reason,
			}
			if term := cs.LastTerminationState.Terminated; term != nil {
				f.ExitCode = term.ExitCode
				f.LastState = "Terminated"
				if term.Reason != "" {
					f.LastState = "Terminated:" + term.Reason
				}
			}
			out = append(out, f)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RestartCount != out[j].RestartCount {
			return out[i].RestartCount > out[j].RestartCount
		}
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		if out[i].Pod != out[j].Pod {
			return out[i].Pod < out[j].Pod
		}
		return out[i].Container < out[j].Container
	})
	return out
}
