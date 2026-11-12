# k8s-crashloop

[![ci](https://github.com/moveeeax/k8s-crashloop/actions/workflows/ci.yml/badge.svg)](https://github.com/moveeeax/k8s-crashloop/actions/workflows/ci.yml)
[![go](https://img.shields.io/badge/go-1.26%2B-00ADD8)](https://go.dev)
[![license](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

> One command to see every crashlooping pod and why it's dying.

Diagnosing a `CrashLoopBackOff` usually means `kubectl get pods`, spotting the bad
one, `kubectl describe` for the exit code, then `kubectl logs --previous` for the
actual error — repeated per pod, per namespace. `k8s-crashloop` does all of it at
once: it lists every crashlooping container with its restart count, exit code, last
state, and the tail of the previous container's logs, sorted worst-first.

```console
$ k8s-crashloop --all-namespaces --tail 20
NAMESPACE  POD          CONTAINER  RESTARTS  EXIT  LAST STATE
prod       api-7c9d     api        12        137   Terminated:OOMKilled
prod       worker-2f1   worker     4         1     Terminated:Error

2 crashlooping container(s).

--- prod/api-7c9d [api] last logs ---
fatal: out of memory
```

## What it does

- Lists containers in `CrashLoopBackOff` across all or selected namespaces.
- Shows restart count, last exit code, and last terminated state per container.
- Fetches the last N lines of the **previous** container's logs (`--tail`, `0` to skip).
- Sorts by restart count (worst first); colorizes hot restart counts; `--json` for automation.
- `restarts` mode ranks every restarted container, crashlooping or not, and says why it
  last died: OOM kill, liveness/startup probe kill, or application exit.

## Install

```bash
go install github.com/moveeeax/k8s-crashloop@latest
```

Uses your standard kubeconfig (`--kubeconfig` to override); read-only, needs `get`/`list`
on pods and `get` on pods/log. `restarts` also lists events (`list` on events); without
that permission it prints a warning and reports probe kills as `Error`.

## Usage

| Flag | Default | Description |
|------|---------|-------------|
| `-A`, `--all-namespaces` | false | Scan all namespaces |
| `-n`, `--namespace` | — | Scan a single namespace |
| `--tail` | `20` | Previous-container log lines to fetch (`0` to skip) |
| `--json` | false | Emit JSON |
| `--no-color` | false | Disable colored output |
| `--kubeconfig` | — | Path to kubeconfig |

`-A`, `-n`, `--no-color` and `--kubeconfig` also apply to the `restarts` subcommand.

### `restarts`: rank by restart count

```console
$ k8s-crashloop restarts -A --since 24h --top 20
NAMESPACE  POD          CONTAINER  RESTARTS  CAUSE          EXIT  REASON     LAST
prod       api-7c9d     api        12        OOMKilled      137   OOMKilled  4m
prod       web-5f2a     web        6         LivenessProbe  137   Error      2h
prod       worker-2f1   worker     4         Error          1     Error      9h

3 restarted container(s).
```

| Flag | Default | Description |
|------|---------|-------------|
| `--since` | `0` (no limit) | Keep only containers whose last termination finished within this window (`24h`, `90m`) |
| `--sort-by` | `count` | `count` (restart count) or `recent` (last termination time, newest first) |
| `--top` | `0` (all) | Show only the first N rows |
| `-o`, `--output` | `table` | `table`, `wide` (adds owner, node, finish time, waiting reason) or `json` |
| `--by-owner` | false | Sum restarts per owning Deployment, StatefulSet, DaemonSet, Job, or bare pod |

`CAUSE` is one of `OOMKilled`, `LivenessProbe`, `StartupProbe`, `Error` (non-zero
exit), `Completed` (exit 0) or `Unknown` (no last termination recorded). Init
containers are included and marked `(init)`.

Things the API does not tell you directly:

- Restart counts are lifetime totals per pod. `--since` filters on when the container
  last died; it cannot count restarts inside the window.
- The runtime reports a probe kill as exit 137/143 with reason `Error`. The kubelet's
  `Killing` event ("Container X failed liveness probe, will be restarted") is what
  identifies it, and the API server keeps events for one hour by default
  (`--event-ttl`), so older probe kills show as `Error`.
- Pods from a ReplicaSet roll up to their Deployment by stripping the
  `pod-template-hash` suffix from the ReplicaSet name; no extra API calls.

## How it works

The default mode lists pods with `client-go`, then inspects each container status for a
`Waiting.Reason == CrashLoopBackOff`, pulling the restart count and the
`LastTerminationState` exit code. For each hit it fetches the previous container's
logs (`Previous: true`), which is where the actual crash lives — the *current*
container is still starting, so its logs are empty. `restarts` reads the same pod list plus
`Killing` events (server-side `reason=Killing` field selector) and matches a probe-kill
event to a container when it falls between the last run's start and finish times. The
Kubernetes access sits behind `Source`/`LogFetcher`/`EventSource` interfaces, so the detection, sorting, and rendering are
unit-tested against fixtures with no cluster.

## Development

```bash
make build   # go build -o k8s-crashloop .
make test    # go test -race ./...
```

## License

MIT — see [LICENSE](LICENSE).
