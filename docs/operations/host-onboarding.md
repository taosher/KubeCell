# Host Onboarding (Four Commands)

> Companion to `technical-design.md`. Conventions: `KUBECONFIG_HUB` points at the management cluster, `KUBECONFIG_HOST` points at the host cluster.

## 0. Prerequisites (one time, prepared by the platform team)

- Pinned versions (see the design): host K3s `v1.36.3+k3s1`, K3k `v1.2.0`, child K3s `v1.34.2+k3s1`, OCM Hub `v1.3.1`. Do not replace them silently.
- Both clusters can pull required images (configure K3s `registries.yaml` mirrors first and verify with a small image).
- Host nodes carry the profile label and TopoLVM topology labels (re-apply them after reinstalls, labels do not survive a rebuild):
  `kubectl label node <node> hardware.kubecell.io/profile=<profile>`
  `kubectl label node <node> topology.topolvm.io/node=<node> topology.kubernetes.io/zone=<node>`
- The controller image is available to both clusters (matching architecture builds; use fully qualified tags).

## 1. `doctor` (read-only preflight)

```bash
go run ./cmd/kubecell-installer doctor management --bundle <release-dir>
go run ./cmd/kubecell-installer doctor host --bundle <release-dir> \
  --device huawei.com/Ascend910
```

Stop on the first failure. On the host side, focus on: nodes Ready, profile labels present, TopoLVM capacity available, and accelerator resources reported by the Device Plugin.

## 2. `join` (host context)

```bash
KUBECONFIG=<host-kubeconfig> clusteradm join \
  --hub-token "$(clusteradm get token)" \
  --hub-apiserver https://hub.example.com:6443 \
  --cluster-name <cell-name> --wait
```

- Do not run `get token` twice between fetching the token and joining (the older token may expire).
- Always pass an explicit `KUBECONFIG` so the command targets the intended cluster.
- The ManagedCluster name must equal the future Cell name; the generator enforces this.

## 3. `accept` (manual approval on the management plane, mandatory safety gate)

```bash
KUBECONFIG=<hub-kubeconfig> clusteradm accept --clusters <cell-name> --wait
```

## 4. `chart` (install the host chart)

```bash
kubectl create ns kubecell-system  # namespaceCreate is false, create it first
helm install kubecell-host ./charts/kubecell-host \
  --namespace kubecell-system \
  --set image.repository=<controller-repo> \
  --set image.tag=<tag> --set image.pullPolicy=IfNotPresent
```

## 5. Cell / Class (zero lines / six numbers)

```bash
# Cell: generated from live host discovery, you only apply it (always use -n kubecell-system)
go run ./cmd/kubecell-installer host --step=cell --cell <cell-name> \
  --kubeconfig <host-kubeconfig> | grep -v '^#' > cell.yaml
kubectl apply -n kubecell-system -f cell.yaml

# Class: suggest-class derives allocatable capacity, then adjust the numbers down
# to the tenant envelope and set the storage class
go run ./cmd/kubecell-installer suggest-class \
  --cell-file cell.yaml --name <class-name>
```

## 6. Plan -> VirtualCluster (eight-line file)

```yaml
apiVersion: kubecell.io/v1alpha1
kind: VirtualCluster
metadata:
  name: <vc-name>
  namespace: kubecell-system
spec:
  cellRef: {name: <cell-name>}
  classRef: {name: <class-name>}
```

Create an isomorphic Plan first and wait for `Accepted`, then create the VirtualCluster. The first Ready transition typically takes 10-20 minutes while child images are pulled.

## 7. Verification (summary)

- `kubectl -n kubecell-system get cell/vc` shows Ready.
- Fetch credentials: the `kubeconfig.yaml` key of the `kubecell-<vc>-<uid>-admin-kubeconfig` Secret.
- Run a CPU Pod in the child cluster, find the reflected host Pod, and check `status.host.quotaUsed` accounting.
- Ingress: create a standard Ingress with class `kubecell` in the child cluster, then access `http://<ingress>.<vc>.apps.example.com`.

## 8. Troubleshooting (quick reference)

| Symptom | Likely cause | Action |
|---|---|---|
| `clusteradm init` reports success but installs nothing | Wrong kubeconfig context | Re-run with an explicit `KUBECONFIG=` |
| OCM components stuck in ImagePullBackOff | Image not available from the configured mirror | Fix the component PullSpec to a reachable registry |
| Preloaded image not recognized by kubelet | Short tag or incomplete layers | Re-tag with the fully qualified name and verify with `crictl inspecti` |
| Child CoreDNS Pending | Third-party Pod without resources blocked by LimitRange | Covered automatically by the `kubecell-defaults` baseline in the foundation Work |
| K3k agent invalid or server OOM | Default LimitRange conflicts with K3k pod-level reservations | Handled by the host webhook pod-level inheritance; recreate Pods after changing defaults (see design) |
| Child PVC reports no free storage | Missing topology labels | Re-apply the labels from section 0 |
| TopoLVM CSINode has no driver | Custom kubelet root directory not passed to the chart | Pass it with `--set` |
| Traefik :80 unreachable | Conflicting Service or stale host-port binding | Use hostNetwork via values, remove stale load-balancer helpers, clear stale NAT chains |
| Child Ingress returns 503 | Mirrored Endpoints carry a port name that does not match | The controller strips port names; for older clusters, clear the endpoint port name to `""` |
| Volumes remain after VirtualCluster deletion | Orphaned logical volumes after the child API is gone | Remove only cluster-created volumes with `lvremove`, leave the volume group itself intact |
| Zero accelerators reported | Device Plugin registration mismatch | Check plugin Pod logs for registration failures before changing anything |

## Explicitly Out of Scope (see design)

The controller never installs K3k, K3s, TopoLVM, Device Plugins, or CNI; it never auto-approves ManagedClusters; the installer owns the locked bundle and charts never mutate OCM objects.
