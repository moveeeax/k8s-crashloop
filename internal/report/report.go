// Package report renders crashloop findings as a table or JSON.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/moveeeax/k8s-crashloop/internal/analyze"
)

// Table writes a human-readable summary. Restart counts are colorized by severity
// when the writer is a terminal-like sink (color is always emitted; pipe through
// `less -R` or a terminal to see it).
func Table(w io.Writer, findings []analyze.Finding, color bool) {
	if len(findings) == 0 {
		fmt.Fprintln(w, "No CrashLoopBackOff pods found. ✅")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "NAMESPACE\tPOD\tCONTAINER\tRESTARTS\tEXIT\tLAST STATE")
	for _, f := range findings {
		restarts := fmt.Sprintf("%d", f.RestartCount)
		if color {
			restarts = colorize(f.RestartCount) + restarts + reset
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n",
			f.Namespace, f.Pod, f.Container, restarts, f.ExitCode, dash(f.LastState))
	}
	tw.Flush()
	fmt.Fprintf(w, "\n%d crashlooping container(s).\n", len(findings))
	for _, f := range findings {
		if f.Logs != "" {
			fmt.Fprintf(w, "\n--- %s/%s [%s] last logs ---\n%s\n", f.Namespace, f.Pod, f.Container, f.Logs)
		}
	}
}

const reset = "\033[0m"

func colorize(restarts int32) string {
	switch {
	case restarts >= 10:
		return "\033[31m" // red
	case restarts >= 3:
		return "\033[33m" // yellow
	default:
		return "\033[0m"
	}
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// JSON writes findings as a JSON array.
func JSON(w io.Writer, findings []analyze.Finding) error {
	if findings == nil {
		findings = []analyze.Finding{}
	}
	return encode(w, findings)
}

func encode(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
