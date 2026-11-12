package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/moveeeax/k8s-crashloop/internal/analyze"
	"github.com/moveeeax/k8s-crashloop/internal/report"
)

// now is overridable in tests.
var now = time.Now

// Output formats accepted by --output.
const (
	outputTable = "table"
	outputWide  = "wide"
	outputJSON  = "json"
)

func newRestartsCmd(g *globalFlags) *cobra.Command {
	var (
		since   time.Duration
		sortBy  string
		top     int
		output  string
		byOwner bool
	)

	c := &cobra.Command{
		Use:   "restarts",
		Short: "Rank containers by restart count with the cause of the last termination",
		Long: "restarts ranks every container that has restarted, crashlooping or not, by its\n" +
			"restart count. Each row shows the last termination's cause (OOMKilled,\n" +
			"LivenessProbe, StartupProbe, Error, Completed), exit code, reason and finish\n" +
			"time. Probe kills are detected from the kubelet's \"Killing\" events, which the\n" +
			"API server keeps for about an hour by default; older probe kills show as Error.",
		Example: "  k8s-crashloop restarts -A --since 24h --top 20\n" +
			"  k8s-crashloop restarts -n prod --by-owner -o wide\n" +
			"  k8s-crashloop restarts -A --sort-by recent -o json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := analyze.ValidSort(sortBy); err != nil {
				return err
			}
			switch output {
			case outputTable, outputWide, outputJSON:
			default:
				return fmt.Errorf("invalid output %q (want %s, %s or %s)", output, outputTable, outputWide, outputJSON)
			}
			if since < 0 {
				return fmt.Errorf("--since must not be negative")
			}
			if top < 0 {
				return fmt.Errorf("--top must not be negative")
			}

			cl, err := providers(g.kubeconfig)
			if err != nil {
				return err
			}
			ctx := context.Background()
			namespaces := g.namespaces()
			pods, err := cl.Pods(ctx, namespaces)
			if err != nil {
				return err
			}
			// Events only refine the cause; without them (e.g. no RBAC on
			// events) probe kills are reported as Error.
			events, err := cl.KillingEvents(ctx, namespaces)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: cannot list events, probe kills will show as %s: %v\n", analyze.CauseError, err)
				events = nil
			}

			t := now()
			rs := analyze.Restarts(pods, events, since, t)
			w := cmd.OutOrStdout()
			wide := output == outputWide
			color := !g.noColor

			if byOwner {
				owners := analyze.ByOwner(rs)
				analyze.SortOwners(owners, sortBy)
				if top > 0 && len(owners) > top {
					owners = owners[:top]
				}
				if output == outputJSON {
					return report.OwnersJSON(w, owners)
				}
				report.Owners(w, owners, wide, color, t)
				return nil
			}

			analyze.SortRestarts(rs, sortBy)
			if top > 0 && len(rs) > top {
				rs = rs[:top]
			}
			if output == outputJSON {
				return report.RestartsJSON(w, rs)
			}
			report.Restarts(w, rs, wide, color, t)
			return nil
		},
	}

	c.Flags().DurationVar(&since, "since", 0, "only containers whose last termination finished within this window, e.g. 24h (0 = no limit)")
	c.Flags().StringVar(&sortBy, "sort-by", analyze.SortByCount, "sort order: count (restart count) or recent (last termination time)")
	c.Flags().IntVar(&top, "top", 0, "show only the first N rows (0 = all)")
	c.Flags().StringVarP(&output, "output", "o", outputTable, "output format: table, wide or json")
	c.Flags().BoolVar(&byOwner, "by-owner", false, "sum restarts per owning Deployment, StatefulSet, DaemonSet, Job or bare pod")
	return c
}
