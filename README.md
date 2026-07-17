# k8s-crashloop

> One command to see every crashlooping pod and why it's dying.

**Status:** 🚧 In development

## Overview

CLI that summarizes CrashLoopBackOff pods across a cluster with their most recent logs.

## Features

- List pods in CrashLoopBackOff (all or selected namespaces)
- Show restart count, last state and exit code per container
- Fetch the last N lines of previous-container logs
- Colorized summary sorted by restart count
- JSON output for automation

## Stack

Go 1.22, `client-go`, `cobra`, `lipgloss`.

## Usage

```bash
k8s-crashloop --all-namespaces --tail 20
```

## License

MIT
