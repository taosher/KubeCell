# KubeCell CI Plan

**Status:** Current plan, complements `technical-design.md` installation order and doctor checklist.
**Principle:** Every large end-to-end run reinstalls from scratch (Cell uninstall + register + install). There is a single management cluster; all online operations run as production operations.

## 0. Layers

| Layer | Where | Content | Trigger |
| --- | --- | --- | --- |
| L0 Local gates | Workstation / CI runner | `make manifests generate`, `go test ./...`, envtest, chart lint/template, CRD drift guard (`test/manifests`) | Every commit (keep existing GitHub workflows) |
| L1 Installer rehearsal | Workstation | Release bundle build, digest verification, `doctor --dry-run`, plan output review | Every installer/chart/compatibility change |
| L2 Full online test | Management + host clusters | Section 2 flow: uninstall -> register and install -> feature matrix -> evidence | Full loop for installer/OCM/baseline changes or weekly; light loop for controller changes |

> [!NOTE]
> Public GitHub runners cannot reach private lab networks. Run L2 on self-hosted runners inside the lab network or manually following this guide (`workflow_dispatch` plus scheduled triggers only orchestrate; execution stays inside the lab).

## 1. Lab Baseline

Keep your own lab baseline in the gitignored `docs/environment.md` (see `docs/environment.example.md`). Before the first L2 run, record:

### 1.1 Management cluster

| Item | Expected |
| --- | --- |
| Machine | Single-node K3s (management version may differ from host version) |
| OCM Hub | All `cluster-manager` Deployments Available |
| Registrations | No stale ManagedCluster registrations |
| KubeCell | No stale VirtualCluster, Cell, controller, CRD, or RBAC leftovers; `kubecell-system` namespace available for reuse |
| Toolchain | `helm` and `kubectl` configured with explicit kubeconfigs (never rely on `localhost:8080`) |
| OCM version | Matches the pin in `third-party/ocm/version.yaml` |

### 1.2 Host cluster

| Item | Expected |
| --- | --- |
| Machine | Ready nodes with the required architecture |
| K3s | Expected pinned version, nodes Ready |
| K3k | CRDs present, controller Available at the pinned version |
| OCM | klusterlet ready for join, or cleanly removed before re-joining |
| Storage | Expected TopoLVM StorageClass exists (`topolvm.io`, `WaitForFirstConsumer`); volume group has free space |
| Host components | Webhook Deployment/Service and webhook configuration present after install |
| Image chain | Registry mirrors configured in `registries.yaml`, K3s restarted, verified with a small uncached image through the CRI path |

### 1.3 Pre-L2 fixes (in order)

1. Configure and verify image mirrors on both clusters.
2. Restore storage health (no degraded controllers, no stale namespaces).
3. Keep K3k upgrades inside the pinned installer baseline; do not upgrade it manually.
4. Verify the OCM version matches the pin before starting.
5. Clear test namespaces and reclaim test volumes.
6. Pass the Section 2 Phase 0 gates before starting.

## 2. L2 Standard Flow (Full Cell Reinstall Loop)

Naming convention: ManagedCluster/Cell uses a stable CI name (e.g. `cell-ci`); Class `ci-small`/`ci-npu`; VirtualCluster `ci-child-<n>`/`ci-npu-<n>`; evidence under `.out/ci-<UTC-timestamp>/` (gitignored).

### Phase 0 Entry gates (do not start if any fail)

Management context: Hub Deployments Available at pinned images, ManagedCluster list healthy with fresh leases, no stuck test leftovers.
Host context: `doctor host` fully passes, especially lvmd, Device Plugin, K3k, and controller health, plus enough free volume-group space.

### Phase 1 Uninstall (deepest first, full by default)

- 1a Delete test VirtualClusters: `kubectl delete virtualcluster` -> `wait --for=delete` -> verify instance/foundation Works, published Secrets, host namespaces, and reflected objects are gone; under `Retain`, PVs become Released and volumes are reclaimed manually with a reclamation record.
- 1b Uninstall the host chart: `helm uninstall kubecell-host`, verify the webhook configuration and Deployment are gone.
- 1c OCM leave: run leave on the host, clean up klusterlet, delete the ManagedCluster and its namespace on the management plane, verify agent namespaces are gone.
- 1d Retire manually installed K3k if it is not Helm-managed: delete the controller and K3k CRDs (Cluster CRs must already be empty after VirtualCluster cleanup) so the installer can reinstall the pinned version.
- Light loop (controller-only changes): skip 1c/1d, keep OCM registration and K3k, only reinstall charts and recreate Cell CRs. Run the full loop for installer/OCM/baseline changes and at least weekly.

### Phase 2 Register and install (design order, verify each step)

`doctor` -> `join` -> manual `accept` -> `chart` -> baseline install (pinned K3k/TopoLVM/Traefik/Device Plugin) -> `host --step=cell` generates and applies the Cell -> Cell Ready gate: Joined/Available/fresh lease/fresh inventory all true, `status.nodes` and inventory match the live host.

### Phase 3 Feature matrix

| Order | Case | Pass criteria | Design reference |
| --- | --- | --- | --- |
| 1 | Plan preflight | `decision=Accepted` | Plan API |
| 2 | Create VirtualCluster | `phase=Ready`, logical node Ready | User story 1 |
| 3 | kubeconfig | Exported Secret works, `get nodes` shows the single `kubelet` | User story 1 |
| 4 | CPU Pod | Child Pod completes, reflected host Pod exists with quota accounting | E2E-001 |
| 5 | Accelerator (single card) | One card allocates and is returned after release | E2E-002 |
| 6 | PVC | Small volume binds, reads/writes, survives server recreation | E2E-003 |
| 7 | Networking | Pod IP, ClusterIP, same- and cross-namespace DNS | NET-001..004 |
| 8 | Interaction | `logs`, valid `exec`, and `attach` pass; document `port-forward` limits explicitly | E2E-004 |
| 9 | hostPath rejection | Pod with hostPath is rejected with the replacement guidance | Host admission |
| 10 | Ingress | Named and numeric ports both return 200 through `<ingress>.<vc>.<apps-suffix>` | Ingress design |
| 11 | Cascading deletion | Deleting the VirtualCluster converges management/OCM/host/child objects to empty, `Retain` data preserved | Deletion semantics |

### Phase 4 Evidence and cleanup

Keep raw output under `.out/ci-<timestamp>/` (never committed); after success, clean up test objects per Phase 1 and keep the volume reclamation record; on failure, snapshot Cell/VirtualCluster YAML, Work status, and host events before cleanup.

## 3. Storage Budget (hard constraint)

Example budget for a 40G volume group: single volume <=2Gi, cases run serially, reclaim after every case; stop and reclaim when free space drops below 10G. Never run two PVC-bearing VirtualClusters in parallel. If logical volumes remain after VirtualCluster deletion (child API already gone), remove only cluster-created volumes with `lvremove`.

## 4. Cadence

L0 on every commit; L1 on installer changes; L2 full loop at least weekly plus on major changes, light loop on controller changes. Do not reinstall the OCM Hub itself as part of routine testing; the `kubecell-management` chart can be reinstalled freely.
