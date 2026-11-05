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

## Install

```bash
go install github.com/moveeeax/k8s-crashloop@latest
```

Uses your standard kubeconfig (`--kubeconfig` to override); read-only, needs `get`/`list`
on pods and `get` on pods/log.

## Usage

| Flag | Default | Description |
|------|---------|-------------|
| `-A`, `--all-namespaces` | false | Scan all namespaces |
| `-n`, `--namespace` | — | Scan a single namespace |
| `--tail` | `20` | Previous-container log lines to fetch (`0` to skip) |
| `--json` | false | Emit JSON |
| `--no-color` | false | Disable colored output |
| `--kubeconfig` | — | Path to kubeconfig |

## How it works

It lists pods with `client-go`, then inspects each container status for a
`Waiting.Reason == CrashLoopBackOff`, pulling the restart count and the
`LastTerminationState` exit code. For each hit it fetches the previous container's
logs (`Previous: true`), which is where the actual crash lives — the *current*
container is still starting, so its logs are empty. The Kubernetes access sits behind
`Source`/`LogFetcher` interfaces, so the detection, sorting, and rendering are
unit-tested against fixtures with no cluster.

## Development

```bash
make build   # go build -o k8s-crashloop .
make test    # go test -race ./...
```

## License

MIT — see [LICENSE](LICENSE).
