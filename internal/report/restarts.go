package report

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/moveeeax/k8s-crashloop/internal/analyze"
)

// Restarts writes the per-container restart ranking. wide adds the owner, node,
// absolute finish time, and current waiting reason.
func Restarts(w io.Writer, rs []analyze.Restart, wide, color bool, now time.Time) {
	if len(rs) == 0 {
		fmt.Fprintln(w, "No restarted containers found.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	header := "NAMESPACE\tPOD\tCONTAINER\tRESTARTS\tCAUSE\tEXIT\tREASON\tLAST"
	if wide {
		header += "\tOWNER\tNODE\tFINISHED\tWAITING"
	}
	fmt.Fprintln(tw, header)
	for _, r := range rs {
		container := r.Container
		if r.Init {
			container += " (init)"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s",
			r.Namespace, r.Pod, container, restartCell(r.RestartCount, color), r.Cause,
			r.ExitCode, dash(r.Reason), age(r.FinishedAt, now))
		if wide {
			fmt.Fprintf(tw, "\t%s\t%s\t%s\t%s", r.Owner, dash(r.Node), timestamp(r.FinishedAt), dash(r.Waiting))
		}
		fmt.Fprintln(tw)
	}
	tw.Flush()
	fmt.Fprintf(w, "\n%d restarted container(s).\n", len(rs))
}

// Owners writes restart totals per workload. wide adds the absolute time of the
// most recent termination.
func Owners(w io.Writer, owners []analyze.OwnerRestarts, wide, color bool, now time.Time) {
	if len(owners) == 0 {
		fmt.Fprintln(w, "No restarted containers found.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	header := "NAMESPACE\tOWNER\tPODS\tCONTAINERS\tRESTARTS\tCAUSES\tLAST"
	if wide {
		header += "\tFINISHED"
	}
	fmt.Fprintln(tw, header)
	for _, o := range owners {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\t%s\t%s",
			o.Namespace, o.Owner, o.Pods, o.Containers, restartCell(o.Restarts, color),
			causes(o.Causes), age(o.FinishedAt, now))
		if wide {
			fmt.Fprintf(tw, "\t%s", timestamp(o.FinishedAt))
		}
		fmt.Fprintln(tw)
	}
	tw.Flush()
	fmt.Fprintf(w, "\n%d workload(s) with restarts.\n", len(owners))
}

// RestartsJSON writes restarts as a JSON array.
func RestartsJSON(w io.Writer, rs []analyze.Restart) error {
	if rs == nil {
		rs = []analyze.Restart{}
	}
	return encode(w, rs)
}

// OwnersJSON writes per-workload totals as a JSON array.
func OwnersJSON(w io.Writer, owners []analyze.OwnerRestarts) error {
	if owners == nil {
		owners = []analyze.OwnerRestarts{}
	}
	return encode(w, owners)
}

func restartCell(n int32, color bool) string {
	s := fmt.Sprintf("%d", n)
	if color {
		return colorize(n) + s + reset
	}
	return s
}

// causes renders a cause histogram as "Error:2,OOMKilled:1", sorted by name.
func causes(m map[string]int) string {
	if len(m) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s:%d", k, m[k])
	}
	return strings.Join(parts, ",")
}

// age renders the time since t in kubectl's short form (45s, 12m, 3h, 2d).
func age(t *time.Time, now time.Time) string {
	if t == nil {
		return "-"
	}
	d := now.Sub(*t)
	switch {
	case d < 0:
		return "0s"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func timestamp(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}
