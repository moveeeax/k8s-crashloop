# examples

`k8s-crashloop` reads a live cluster via your kubeconfig — there's nothing to run
offline. Common invocations:

```bash
# every crashlooping pod in the cluster, with 20 lines of previous logs
k8s-crashloop --all-namespaces --tail 20

# one namespace, JSON for automation
k8s-crashloop -n prod --json

# just the table, no log fetch
k8s-crashloop -A --tail 0

# containers that restarted in the last day, worst 20
k8s-crashloop restarts -A --since 24h --top 20

# restarts summed per Deployment/StatefulSet/DaemonSet, with finish times
k8s-crashloop restarts -n prod --by-owner -o wide
```
