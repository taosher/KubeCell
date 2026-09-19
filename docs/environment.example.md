# Environment Notes (Template)

> Copy this file to `docs/environment.md` for local use. `docs/environment.md` is gitignored and never committed.

## Topology

| Role | Address (example) | Notes |
| --- | --- | --- |
| Management cluster | `https://hub.example.com:6443` | Single-node K3s, hosts OCM Hub and the KubeCell management controller |
| Cell host cluster | `https://host-01.example.com:6443` | Host cluster backing one Cell, runs klusterlet, K3k, TopoLVM, Device Plugin |

Constraints:

- Record only your own lab addresses in the local copy. Do not commit real IPs, SSH users, or credentials.
- Keep host preparation (K3s install, labels, volume groups) in your own runbook. The installer only consumes versioned manifests.
- If public image registries are unreachable from your lab, configure K3s `registries.yaml` mirrors first and verify with a small uncached image before running workloads.

## K3s Registry Mirrors (Example)

```yaml
mirrors:
  "docker.io":
    endpoint: ["https://your-mirror.example/docker"]
  "ghcr.io":
    endpoint: ["https://your-mirror.example/ghcr"]
  "registry.k8s.io":
    endpoint: ["https://your-mirror.example/k8s"]
  "quay.io":
    endpoint: ["https://your-mirror.example/quay"]
```

Restart K3s after changing mirrors and verify the chain with a small test image. If a node uses an embedded registry cache, list it first so cache misses fall back to the mirrors.
