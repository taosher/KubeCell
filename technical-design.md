# KubeCell Technical Design (Shared Mode)

**Status:** Current authoritative design. Where behavior differs from any other document, this document takes precedence.

## 1. Goals and Non-Goals

### 1.1 Goals

On shared physical hosts, provision mutually isolated Kubernetes child clusters (VirtualClusters) for developers, with the following requirements:

- Each child cluster owns an independent K3s API server and data plane; developers access it with standard `kubectl`;
- Multiple child clusters share the same physical hosts and accelerators, with hard quota isolation;
- The management-plane controller only writes OCM `ManifestWork` objects and only reads hosts through the OCM proxy; it never connects directly to any host cluster;
- Storage uses host-side TopoLVM; child-cluster PVCs are reflected as host PVCs;
- Accelerators (Ascend/NVIDIA) follow unified Kubernetes extended-resource semantics; no per-vendor branches.

### 1.2 Non-Goals

- No custom scheduler, device allocator, kubelet, CNI, CSI, Device Plugin, or host agent (no-fork policy, see Section 7).
- The first version does no automatic cross-Cell placement, migration, or failover; each VirtualCluster is explicitly bound to one Cell.
- No strong-adversary multi-tenancy model; developer credentials are child-cluster admin kubeconfigs (trusted-network trade-off).
- Ingress routing, `port-forward`, and automatic physical-data reclamation are outside the locked-combination commitments.

## 2. Three-Layer Architecture

```text
Management cluster (single-node K3s)
├── OCM Hub (ManagedCluster / ManifestWork / ManagedServiceAccount / Cluster Proxy)
├── KubeCell management controller + CRD webhook (kubecell-management chart)
│   └── 4 CRDs: Cell / VirtualNodeClass / VirtualClusterPlan / VirtualCluster
├── KubeCell installer (cmd/kubecell-installer, release-bundle orchestration)
└── Writes ──> OCM ManifestWork (two per child cluster)

Cell host cluster (one Cell = one OCM ManagedCluster = one host cluster, ARM64)
├── OCM klusterlet + work-agent (applies ManifestWork)
├── K3k controller v1.2.0 (creates/reflects child clusters)
├── KubeCell host webhook (kubecell-host chart, admission only)
├── Host baseline (not installed by controllers; Kubernetes-side manifests are
│   installed by the installer at pinned versions, see §8): CNI/CoreDNS, TopoLVM,
│   vendor Device Plugins, profile labels
└── One host namespace per child cluster: reflected Pods/PVCs, ResourceQuota, NetworkPolicy

Virtual cluster (child K3s)
├── Independent API server + data volume (K3k PVC on TopoLVM)
├── One logical child node (named kubelet; entitlement view only, not a physical machine)
└── Child-cluster admin kubeconfig (rewritten endpoint, republished to a management-plane Secret)
```

Child-cluster APIs are exposed through host NodePorts: `https://<host-address>:<NodePort>`.

## 3. API and CRD Design

KubeCell owns exactly four public CRDs under `kubecell.io/v1alpha1`: `Cell`, `VirtualNodeClass`, `VirtualClusterPlan`, and `VirtualCluster`. All are desired-state/preflight APIs. The logical child node of a child cluster has no dedicated CRD: readiness, quota, and per-reflected-Pod placement observations are folded into `VirtualCluster.status`; scale-out is not supported — only vertical quota changes (switching to a different Class) are allowed. There are no horizontal fields such as node count, node pools, or replicas. Multiple logical nodes would be a design change (see §10).

Fixed principle: human-authored `spec` expresses only "intent" (which Cell, which profile, how much quota, credentials are always issued). Any field with a single safe value is a platform constant and never enters the CRD. Version-class changes go through releases, never through CR edits.

There is no public `MachineProfile` CRD. Each Cell embeds exactly one machine profile `machineProfile` (a single object, not a list; heterogeneous multi-profile support is deferred beyond the current scope and will be designed separately). One Cell can never accidentally reference another Cell's inventory.

| CRD | Scope | Short name | Owner |
| --- | --- | --- | --- |
| `Cell` | Namespaced | `cell` | Platform |
| `VirtualNodeClass` | Cluster-scoped | `vnclass` | Platform |
| `VirtualClusterPlan` | Namespaced | `vcp` | Tenant/platform |
| `VirtualCluster` | Namespaced | `vc` | Tenant/platform |

### 3.1 Cell (namespaced)

The `spec` has only two groups:

- `managedClusterRef{name}`: the bound OCM `ManagedCluster` name. Once any VirtualCluster has resolved this Cell, the reference is immutable. This is the Cell's only "binding declaration".
- `machineProfile{name, devices[]}`: the Cell's single machine profile. Multiple physical machines of the same profile may exist; all count as inventory. The profile name and device contracts become immutable once any VirtualCluster has resolved them.

The API address is not part of `spec` and is discovered automatically by the controller: it reads Ready host nodes through the proxy, sorts them by node name, takes the InternalIP of the first node, records it as the API address of each VirtualCluster in `status.endpoint.address`, and republishes kubeconfigs when the address changes. There are no other `spec` fields beyond these two groups. Exposure method, port range, versions, and proxy identity are all platform constants (see §7).

The device contract `devices[]` has only two fields. There is no separate `devicePluginProfile` field and no public CRD for it:

| Field | Meaning |
| --- | --- |
| `name` | Stable identifier inside KubeCell, used when checking profile device entitlements |
| `resourceName` | Exact host extended-resource key; child-cluster Pods and reflected host Pods must use the identical key |

The allocation semantic is always exclusive whole-card allocation (request equals limit and is an integer), enforced by the controller; no additional field is needed.

The contract is a declarative authorization, not a reservation. `runtimeClassName` is deliberately absent: developers may omit it in child-cluster workload Pod templates or fill it in according to the host-baseline convention. KubeCell never requires, defaults, infers from `resourceName`, or rejects device requests because of it. The child-cluster workload YAML is the sole source of that field. The host platform layer is responsible for installing and upgrading Device Plugins, vendor runtimes, and RuntimeClasses. The controller only checks matched nodes: the node must be `Ready` and the `capacity`/`allocatable` of `resourceName` must be positive. The observed allocatable is the source of fact; the Cell declaration is an upper bound and must never inflate capacity (if 8 cards are declared but only 4 are observed, the effective capacity is 4 and a `CapabilityMismatch` condition is reported). KubeCell does not check Device Plugin Pods, RuntimeClasses, vendor drivers/runtimes, CDI, or vendor health; related diagnostics go through host Pod status, Events, and logs.

`status` fields: `observedGeneration`; `phase` (`Pending`/`Provisioning`/`Ready`/`Degraded`/`Deleting`/`Failed`); `conditions`; `managedCluster{name, uid, joined, available, leaseFresh, leaseRenewTime}`; `provider{k3kVersion, namespaceReady, k3kCRDsReady, controllerReady, cniReady, topolvmReady}`; `nodes[]{name, profile, ready, labelHash, capacity, allocatable, activeRequested, pendingRequested, readinessReasons, observedAt}`; `inventory` (resource name to `capacity`/`allocatable`/`requested`/`pending`/`availableEstimate`); `storageInventory` (`provisioner`, `allocatedPV`, per-node `allocatedPV`/`freeCapacity`/`freeCapacityKnown`/`freeCapacitySource`/`fresh`, aggregated free space and source, observation time, freshness); `nodePorts{allocated, observedAt, fresh}`. TopoLVM free space must come from capacity reported by TopoLVM/lvmd with a live source; inferring it from PV capacity is forbidden. When a required inventory source is unavailable or stale, the Cell must not report inventory Ready.

A Cell is usable if and only if the OCM conditions hold with `Joined=True` and `Available=True`, and the host Lease `managed-cluster-lease` is fresh. `Available=True` alone is not trustworthy (under the observed OCM combination this condition may go stale without updates); Lease freshness is the decisive liveness criterion.

Full example (`status` is filled in by the controller as shown; do not hand-write it):

```yaml
apiVersion: kubecell.io/v1alpha1  # Fixed: KubeCell-owned API group and version, not a Kubernetes native resource
kind: Cell                        # Fixed: Cell type, namespaced, platform-owned
metadata:
  name: cell1                     # Cell name; by convention identical to the OCM ManagedCluster name, enforced by the generator
  namespace: kubecell-system      # Fixed namespace: Cells must live in the same namespace as the controller
spec:
  managedClusterRef:
    name: cell1                   # Required: bound OCM ManagedCluster name; immutable once resolved by any VirtualCluster
  machineProfile:                 # The Cell's single machine profile (single object; multiple physical machines of the same profile all count as inventory)
    name: ascend-910b             # Profile name; immutable once resolved; the placement selector is this name
    devices:                      # Inline accelerator contract; no standalone CRD
    - name: ascend                # Stable identifier inside KubeCell, referenced by entitlement checks
      resourceName: huawei.com/Ascend910  # Exact host extended-resource key; must match character-for-character in all six places
status:                               # Everything below is filled in by the controller, read-only; do not hand-write
  observedGeneration: 1               # Observed metadata.generation; used to tell stale from fresh
  phase: Ready                        # Pending/Provisioning/Ready/Degraded/Deleting/Failed
  conditions: []                      # Standard condition table: Joined/Available/ProviderReady/InventoryFresh and others
  managedCluster:                     # Observation of the OCM registered object
    name: cell1                       # ManagedCluster name; must match managedClusterRef
    uid: "..."                        # ManagedCluster UID; compared at UID level on deletion
    joined: true                      # OCM Joined condition; false means Degraded
    available: true                   # OCM Available condition; not trustworthy alone, must be combined with leaseFresh
    leaseFresh: true                  # Whether managed-cluster-lease is fresh; the true liveness criterion
    leaseRenewTime: "2026-09-12T00:00:00.000000Z"  # Last Lease renewal time (microsecond precision)
  provider:                           # Decomposition of host-baseline readiness
    k3kVersion: v1.2.0                # Observed K3k version; must equal the locked combination (§7), drift raises an alert
    namespaceReady: true              # k3k-system namespace exists
    k3kCRDsReady: true                # K3k CRDs (Cluster/Policy) exist
    controllerReady: true             # K3k controller Deployment is ready
    cniReady: true                    # Host CNI is ready
    topolvmReady: true                # Host TopoLVM is ready
  nodes:                              # Per-node observations; nodes without a profile label are reported and skipped
  - name: host-01                     # Host node name
    profile: ascend-910b              # Matched profile pool name, determined by the profile label
    ready: true                       # Node Ready condition
    labelHash: "..."                  # Label snapshot hash; label changes are detectable
    capacity: {cpu: "192", memory: "512Gi", huawei.com/Ascend910: "8"}      # Node-reported totals
    allocatable: {cpu: "190", memory: "500Gi", huawei.com/Ascend910: "8"}   # Allocatable after system reservations; source of fact
    activeRequested: {cpu: "4", memory: 8Gi, huawei.com/Ascend910: "2"}     # Sum of running Pod requests
    pendingRequested: {cpu: "0", memory: 0Gi, huawei.com/Ascend910: "0"}    # Sum of pending Pod requests
    readinessReasons: []              # Not-ready reason codes; empty means no anomaly
    observedAt: "2026-09-12T00:00:00Z"  # Snapshot time of this node
  inventory:                          # Aggregated inventory: resource name to five quantities
    cpu: {capacity: "192", allocatable: "190", requested: "4", pending: "0", availableEstimate: "186"}  # capacity is the total; allocatable is assignable; requested is used; pending is awaiting scheduling; availableEstimate is the estimated remainder
  storageInventory:                   # TopoLVM inventory; free space must come from lvmd reports, never inferred from PVs
    topolvm-provisioner:              # Key is the host StorageClass name
      provisioner: topolvm.io         # Provisioner; must be topolvm.io
      allocatedPV: 100Gi              # Total allocated PVs (Kubernetes-observed side)
      freeCapacity: 900Gi             # Physical free space reported by lvmd; key absent when unknown
      freeCapacityKnown: true         # Whether free space is known; inventory must not report Ready when false
      freeCapacitySource: topolvm-lvmd  # Free-space source identifier; for auditing
      fresh: true                     # Whether this StorageClass inventory is fresh
      observedAt: "2026-09-12T00:00:00Z"  # Observation time
  nodePorts:                          # NodePort allocation observations
    allocated: [31076]                # Observed allocated port table
    fresh: true                       # Whether the port observation is fresh
    observedAt: "2026-09-12T00:00:00Z"  # Observation time
```

### 3.2 VirtualNodeClass (cluster-scoped)

The `spec` has only two groups: `entitlement{workloadHard}` (quota numbers, the only thing a human must fill in) and `storageClassName` (storage name). The `status` carries only `observedGeneration` and `conditions`.

This object is an administrator policy object: one name maps to one quota tier. Every extended-resource key in `entitlement.workloadHard` must be declared by the selected Cell's single profile and must not exceed the effective allocatable capacity observed on the Cell. A Class never installs, configures, or renames Device Plugin resources. Once referenced by any VirtualCluster, a Class is immutable as a whole; to adjust quotas, create a new Class.

Apart from quotas and the storage name, all other behavior is platform constants: the execution mode is always Shared mode; the child-cluster version comes from the locked constants (see §7); the K3k overhead reservation is always control plane `500m/1Gi` plus reflected system `100m/256Mi`; the host-namespace container resource default is always `100m/256Mi` (the foundation Work ships a LimitRange that backfills third-party Pods without declared resources, such as the K3k built-in coredns and the dashboard session proxy; verified online). Value constraint: the default must not exceed the K3k reflected-system reservation (`100m/256Mi`); otherwise K3k agents (pod-level reservation plus empty containers) would be burst by the defaults into illegal Pods (identified through online diagnosis); it shares one constant with the reflected-system reservation and equals it. The placement label key is always `hardware.kubecell.io/profile`; storage binding is always `WaitForFirstConsumer` and reclamation always `Retain`; the security baseline is always baseline, non-privileged, no host namespace/network, and hostPath volumes are always rejected.

Full example:

```yaml
apiVersion: kubecell.io/v1alpha1  # Fixed group and version
kind: VirtualNodeClass            # Fixed type; cluster-scoped; platform-owned; short name vnclass
metadata:
  name: ascend-910b               # Class name; referenced by VirtualCluster classRef; immutable as a whole once referenced
  # Note: cluster-scoped resource, no metadata.namespace
spec:
  entitlement:
    workloadHard:                 # Hard entitlement limits; each entry becomes host ResourceQuota; request must equal limit
      requests.cpu: "4"           # Hard CPU request ceiling; string quantity format
      limits.cpu: "4"             # CPU limit; must equal requests.cpu
      requests.memory: 8Gi        # Hard memory request ceiling
      limits.memory: 8Gi          # Memory limit; must equal requests.memory
      requests.huawei.com/Ascend910: "2"  # Accelerator request; key must be declared by the Cell profile devices; integer
      limits.huawei.com/Ascend910: "2"    # Accelerator limit; must equal the corresponding requests entry
  storageClassName: topolvm-provisioner  # Must be an accredited TopoLVM class; binding/reclamation are fixed on the rendering side
```

### 3.3 VirtualClusterPlan (namespaced)

`spec.virtualCluster` is a by-value copy, not a reference to an existing `VirtualCluster`; it goes through the same validation and inventory-evaluation logic as a real resource.

`status` fields: `observedGeneration`; `phase`/`decision` (`Accepted`/`Rejected`/`Unknown`); `checks[]` (at most 16 entries, each with `name` (at most 64 characters, stable name), `status` (`Passed`/`Failed`/`Unknown`), `reason` (at most 128), `message` (at most 1024), `observedAt`; required checks cover Cell/OCM availability and Lease freshness, host baseline, locked versions, CPU/memory/ephemeral storage, NPU/GPU allocatable capacity and affinity, host quota, TopoLVM physical capacity and affinity, NodePort availability); `resolved` (non-secret summary: Cell UID, ManagedCluster, profile, profile hash, child-cluster version, effective compute/device/storage requests, observation timestamp); `inventoryObservedAt`, `inventoryFresh`, `expiresAt`.

Decision formula: `Rejected` means at least one check `Failed`; `Unknown` means no `Failed` but at least one required check is `Unknown`; `Accepted` means all required checks `Passed`. `Accepted` only describes an observation snapshot, not a reservation or scheduling priority; an expired conclusion must never be reused.

Evaluation timing: a Plan is evaluated exactly once, on creation and on spec changes; the controller performs no periodic refresh and no requeue loop. `expiresAt` is an advisory expiry (evaluation time plus 5 minutes); an expired conclusion is void, and re-applying yields a fresh evaluation. The recheck before VirtualCluster creation is the only hard gate; no Plan conclusion can substitute for it. This API serves external systems: apply a Plan, poll `status.decision`, then decide whether to create a VirtualCluster, all through the standard Kubernetes API.

The Plan controller is read-only toward hosts: it may only write `VirtualClusterPlan/status` and plain management-plane Events. It must never create or update `VirtualCluster` objects, `ManifestWork` objects, host namespaces, quotas, K3k clusters, Services, Secrets, PVCs, child-cluster resources, or NodePort allocations or reservations. Deleting a Plan performs no host cleanup.

The real `VirtualCluster` reconciler must rerun the same shared evaluator after resolving the Cell/Class/profile and before creating the foundation Work: if the conclusion is not `Accepted`, it must not create provisioning Works and must report the checks to `VirtualCluster.status.feasibilityChecks`. An old Accepted conclusion may be cited for auditing but the recheck is never skipped, which closes the "capacity was consumed by someone else after the Plan was accepted" race.

Full example (`status` is filled in by the controller as shown; do not hand-write it):

```yaml
apiVersion: kubecell.io/v1alpha1  # Fixed group and version
kind: VirtualClusterPlan          # Fixed type; namespaced; short name vcp; preflight only
metadata:
  name: child-dev-plan            # Plan name; independent of VirtualCluster names, citable for auditing
  namespace: kubecell-system      # Must be created in kubecell-system (namespace rules, see 3.4)
spec:
  virtualCluster:                 # By-value copy, isomorphic to VirtualClusterSpec; not a reference
    cellRef: {name: cell1}        # Candidate Cell; must be Ready
    classRef: {name: ascend-910b} # Candidate Class; must exist
status:                           # Everything below is filled in by the controller, read-only
  observedGeneration: 1           # Observed generation
  phase: Accepted                 # Same value as decision; Accepted/Rejected/Unknown
  decision: Accepted              # Final conclusion: Accepted means all passed; Rejected means a failure; Unknown means unknowns remain
  checks:                         # Per-item checks; at most 16 entries; sorted by name; bounded
  - name: CellAvailable           # Stable check name: Cell availability
    status: Passed                # Passed/Failed/Unknown
    reason: ManagedClusterAvailable  # Machine-readable reason code (at most 128 characters)
    message: ""                   # Human-readable supplement (at most 1024 characters); empty when healthy
    observedAt: "2026-09-12T00:00:00Z"  # Time of this check
  - name: TopoLVMCapacity         # Stable check name: TopoLVM capacity and affinity
    status: Passed                # Same three states
    reason: CapacitySufficient    # Capacity-sufficient reason code
    message: ""                   # Supplement
    observedAt: "2026-09-12T00:00:00Z"  # Check time
  resolved:                       # Non-secret summary; no credentials, no host object dumps, no logs
    cellUID: "..."                # Resolved Cell UID
    managedClusterName: cell1     # Resolved ManagedCluster name
    classUID: "..."               # Resolved Class UID
    profileHash: "..."            # Profile snapshot hash; used for drift detection
    machineProfileName: ascend-910b  # Matched profile pool name
    k3kVersion: v1.2.0        # Resolved K3k version
    childK3sVersion: v1.34.2+k3s1 # Resolved child version (public form)
    workloadRequests: {cpu: "4", memory: 8Gi, huawei.com/Ascend910: "2"}  # Effective request totals
    reservation: {cpu: 600m, memory: 1280Mi}  # Total reservations (control plane plus reflected system)
    storageClassName: topolvm-provisioner  # Resolved storage class name
  inventoryObservedAt: "2026-09-12T00:00:00Z"  # Snapshot time of the inventory used; not the evaluation time
  inventoryFresh: true            # Whether the inventory used was fresh; conclusions are untrustworthy when false
  expiresAt: "2026-09-12T00:05:00Z"  # Conclusion expiry time; expired conclusions must not be reused and require a recheck
```

### 3.4 VirtualCluster (namespaced)

The `spec` has only two groups: `cellRef{name}` (target Cell, namespace always `kubecell-system`) and `classRef{name}` (entitlement profile). The child-cluster admin kubeconfig is always published; there is no toggle.

Immutability rules: `cellRef` and `classRef` are immutable after creation.

Namespace rule: a VirtualCluster must be created in `kubecell-system`; the webhook rejects all other namespaces (the same applies to Cell and VirtualClusterPlan).

`status` fields: `observedGeneration`; `phase` (`Pending`/`Provisioning`/`Ready`/`Degraded`/`Deleting`/`Failed`); `conditions`; `resolved` (Cell UID, ManagedCluster name, Class UID, profile hash, profile name, K3k version, child-cluster version, K3k child version, child image tag, derived host namespace, Work names, quota, reservations, accelerator keys, storage mapping; the sole basis for steady-state rendering); `workRefs{foundation, instance}` (each with `namespace`/`name`/`uid`/`applied`/`progressing`/`failed`); `endpoint{address, nodePort, urlHash}`; `host{namespace, quotaReady, storageReady, networkReady, inventoryFresh, quotaHard, quotaUsed, tenantHeadroom, reservation, reservationExceeded, k3kPhase}`; `child{apiReady, logicalNodeName, logicalNodeReady, storageClassReady}`; `credential{secretName, sourceSecretName, endpointHash, observedResourceVersion}` (records only Secret names, source name, endpoint hash, and resource version; never kubeconfig bytes or tokens); `feasibilityChecks[]` (recheck report at creation); `placements[]{childUID, hostUID, phase, hostNode, requests, pvNode, failureLayer, observedAt}` (per-reflected-Pod placement observations: child/host Pod UID pair, phase, actual physical node, resource requests, PV node, failure layer).

Ready requires: both Works Applied, K3k Ready, child-cluster API reachable, logical node Ready, and the mapped child-cluster StorageClass present.

Full example (`status` is filled in by the controller as shown; do not hand-write it):

```yaml
apiVersion: kubecell.io/v1alpha1  # Fixed group and version
kind: VirtualCluster              # Fixed type; namespaced; short name vc; the product itself
metadata:
  name: child-cell1-dev           # Child-cluster name; derived names build on it
  namespace: kubecell-system      # Fixed namespace
spec:
  cellRef: {name: cell1}          # Required: target Cell; immutable after creation; namespace always kubecell-system
  classRef: {name: ascend-910b}   # Required: entitlement profile; immutable after creation
status:                           # Everything below is filled in by the controller, read-only
  observedGeneration: 1           # Observed generation
  phase: Ready                    # Pending/Provisioning/Ready/Degraded/Deleting/Failed
  conditions: []                  # Standard conditions: FoundationReady/InstanceReady/Validated/Credential and others
  resolved:                       # Resolution snapshot; sole basis for steady-state rendering; frozen after creation
    cellUID: "..."                # Cell UID; compared at UID level on deletion
    managedClusterName: cell1     # Target ManagedCluster name; both Works are created in this namespace
    classUID: "..."               # Class UID
    profileHash: "..."            # Profile snapshot hash; drift reports ResolvedDrift
    machineProfileName: ascend-910b  # Profile pool name
    k3kVersion: v1.2.0        # Locked K3k version
    childK3sVersion: v1.34.2+k3s1 # Child version in public form
    k3kChildVersion: v1.34.2-k3s1 # Child version in K3k syntax (hyphen); translated centrally by provider/profile
    childImageTag: v1.34.2-k3s1   # Child image tag; same as above
    hostNamespace: kc-child-cell1-dev-3f54a1b2  # Derived host namespace; embeds an 8-character UID prefix against collisions
    k3kClusterName: kc-child-cell1-dev-3f54a1b2  # Derived K3k Cluster name; same as above
    quota: {requests.cpu: "4", limits.cpu: "4", requests.memory: 8Gi,  # Host quota expectation; from Class workloadHard
      limits.memory: 8Gi, requests.huawei.com/Ascend910: "2",
      limits.huawei.com/Ascend910: "2"}
    reservation:                  # Reservation snapshot; from the Class
      policy: IncludedInHostQuota  # Reservation policy
      controlPlane: {cpu: 500m, memory: 1Gi}       # Control-plane reservation
      reflectedSystem: {cpu: 100m, memory: 256Mi}  # Reflected-system reservation
    acceleratorKeys: [huawei.com/Ascend910]  # Accelerator key table; used by the admission allowlist
    storage:                      # Storage mapping snapshot; from the Class
      class:
        childName: topolvm-provisioner      # Child-side class name
        hostName: topolvm-provisioner       # Host-side class name; must be equal
        volumeBindingMode: WaitForFirstConsumer  # Binding mode
        reclaimPolicy: Retain               # Reclamation policy
        allowVolumeExpansion: true          # Expansion toggle
  workRefs:                       # References to the two Works and delivery observations
    foundation: {namespace: cell1, name: kc-3f54a1b2-foundation,  # Foundation Work: namespace/quota/policy
      uid: "...", applied: true, progressing: false, failed: false}  # UID ownership; Applied/Progressing/Failed tri-state
    instance: {namespace: cell1, name: kc-3f54a1b2-instance,  # Instance Work: K3k Cluster/Policy
      uid: "...", applied: true, progressing: false, failed: false}
  endpoint: {address: 203.0.113.10, nodePort: 31076, urlHash: "..."}  # Auto-discovered host address (first Ready node IP); confirmed port; URL hash
  host:                           # Host-side observations
    namespace: kc-child-cell1-dev-3f54a1b2  # Host namespace; must match resolved.hostNamespace
    quotaReady: true              # Host quota is ready
    storageReady: true            # Host storage is ready
    networkReady: true            # Host network policy is ready
    inventoryFresh: true          # Inventory used is fresh
    quotaHard: {requests.cpu: "4", limits.cpu: "4", requests.memory: 8Gi,  # Measured host quota hard limits
      limits.memory: 8Gi, requests.huawei.com/Ascend910: "2",
      limits.huawei.com/Ascend910: "2"}
    quotaUsed: {requests.cpu: "2", limits.cpu: "2", requests.memory: 4Gi,  # Measured host quota usage
      limits.memory: 4Gi, requests.huawei.com/Ascend910: "1",
      limits.huawei.com/Ascend910: "1"}
    tenantHeadroom: {requests.cpu: "2", limits.cpu: "2", requests.memory: 4Gi,  # Tenant headroom equals hard limits minus used
      limits.memory: 4Gi, requests.huawei.com/Ascend910: "1",
      limits.huawei.com/Ascend910: "1"}
    reservation: {cpu: 600m, memory: 1280Mi}  # Measured total reservations
    reservationExceeded: false     # Whether reservations are exceeded; true means Degraded
    k3kPhase: Ready               # K3k Cluster phase
  child: {apiReady: true, logicalNodeName: kubelet,  # Child API reachable; logical node name is always kubelet
    logicalNodeReady: true, storageClassReady: true}  # Logical node ready; mapped StorageClass ready
  credential:                     # Credential publication record; names and hashes only, never credential bytes or tokens
    secretName: kubecell-child-cell1-dev-3f54a1b2-admin-kubeconfig  # Management-plane published Secret; name embeds UID against collisions
    sourceSecretName: k3k-kc-child-cell1-dev-3f54a1b2-kubeconfig  # Host K3k source Secret
    endpointHash: "..."           # Endpoint hash; endpoint changes are detectable without storing the URL in cleartext
    observedResourceVersion: "12345"  # Source Secret resource version; rotations are detectable
  feasibilityChecks: []           # Recheck report at creation; empty when healthy, otherwise lists check items
```

### 3.5 Relationship to External Resources

The desired direction is always: management-plane CRs to controller to OCM `ManifestWork` in the target ManagedCluster namespace to host resources. The observation direction is: host resources to OCM feedback/proxy reads to status. KubeCell must never directly create reflected host Pods, PVs, or Device Plugin allocations.

| Resource | Location | Owner/writer | Purpose |
| --- | --- | --- | --- |
| `ManagedCluster` | Management plane/OCM Hub | OCM/operations | Registers the Cell host cluster, reports Joined/Available |
| `ManifestWork` | Target ManagedCluster namespace on the management plane | KubeCell controller | Ships foundation/instance desired resources, with bounded feedback |
| klusterlet/work-agent | Cell host | OCM | Pulls and applies Works; no KubeCell code ownership |
| Host namespace | Cell host | work-agent via foundation Work | Per-child-cluster quota/security/reflection boundary |
| Host ResourceQuota/LimitRange | Cell host | work-agent via foundation Work | Host hard resources and defaulting boundary |
| Host NetworkPolicy/PSA labels | Cell host | work-agent via foundation Work | Host network/admission policy |
| K3k `Cluster` | Cell host | work-agent via instance Work, reconciled by the K3k controller | Child-cluster Shared-mode control plane |
| K3k `VirtualClusterPolicy` | Cell host | work-agent via instance Work | Child-side policy projection only, not a host hard quota |
| K3k server StatefulSet/Pod/PVC | Cell host | K3k controller | Child-cluster API server and data volume |
| K3k Service | Cell host | K3k controller | Child-cluster API NodePort endpoint |
| K3k kubeconfig Secret | Cell host | K3k controller | Child-cluster admin kubeconfig source, read through the proxy |
| Child StorageClass | Virtual cluster | Controller via child-cluster credentials | Child-side storage identity mapped to the Cell-wide host TopoLVM class |
| Child Pod/PVC/Service | Virtual cluster | Developer/child-cluster controllers | Tenant workloads, reflected by K3k |
| Reflected host Pod/PVC | Cell host | K3k reflector | Physical execution objects; the management plane never writes them directly |
| Host nodes/Device Plugin/TopoLVM | Cell host | Host platform/GitOps | Physical capacity/allocation/provisioning; KubeCell only observes node resource facts |
| Published management-plane admin kubeconfig Secret | Management plane | KubeCell controller | Developer credential with rewritten endpoint |

### 3.6 Relationship of the Three Concepts and Naming

The only canonical name in this design for the cluster that carries the OCM Hub and the KubeCell management controller is the **Management Cluster**, consistent with the installer targets (`management`/`host`) and chart names (`kubecell-management`/`kubecell-host`). The three concepts relate as follows:

- The management cluster is the sole source of desired state: it carries the OCM Hub, the KubeCell management controller, and Cell/VirtualCluster CRs. It only ships desired state and reads observations; it runs no business workloads and connects directly to no host.
- An OCM `ManagedCluster` is not a third cluster; it is a **registration object** in the OCM Hub on the management cluster, representing one Cell host cluster one-to-one. `Cell.spec.managedClusterRef` binds it by name, and the two `ManifestWork` objects are created in the namespace named after it.
- A Cell host cluster is the physical executor: it runs klusterlet/work-agent, K3k, TopoLVM, Device Plugins, and reflected workloads, accepting management-cluster Works through the klusterlet tunnel.

Call chain: Cell CR (management cluster) to `managedClusterRef` to the `ManagedCluster` object (management-cluster OCM Hub) to the klusterlet tunnel to the Cell host cluster. The terms "management plane", "management cluster", and "Management Cluster" used in this and other current documents all denote the same object; no further synonyms may be introduced.

## 4. Data Flow

Creation: `kubectl apply VirtualCluster` to CRD webhook (validation plus immutability plus global name-duplication check) to VirtualCluster reconciliation: resolve Cell plus Class, run the same evaluation as a Plan, create the foundation Work, create the instance Work, OCM Hub, work-agent applies on the host, K3k creates the server Pod plus PVC (TopoLVM), kubeconfig Secret, NodePort Service, child-cluster API, one logical node, child-cluster workloads are reflected as host Pods (resource requests preserved verbatim), reconciler reads the K3k kubeconfig Secret through the OCM proxy (verifies UID), rewrites the endpoint to the confirmed NodePort, probes the child-cluster API/nodes/StorageClass, publishes the admin kubeconfig Secret to the management plane, user works with `kubectl --kubeconfig`.

Deletion (reverse order): delete the published Secret, delete both Works (UID label check), wait for Work deletion, remove the finalizer. Under `Retain`, PVs become `Released` and PV/LV data is preserved; objects with mismatched ownership are only reported, never deleted.

## 5. Host Admission (Host Webhook)

For Pod creation requests in namespaces carrying the KubeCell label: reject hostNetwork/hostPID/hostIPC/privileged/nodeName pinning/hostPath volumes; enforce the device-resource allowlist, profile node affinity, and storage node constraints. The hostPath rejection message is: `hostPath volumes are not supported in KubeCell shared-mode namespaces; use a PVC with the mapped TopoLVM StorageClass` (no host-file access on shared hosts; all data goes through TopoLVM). Host `ResourceQuota` is the hard limit; the K3k policy quota is not. Infrastructure namespaces (K3k server, TopoLVM, Device Plugin) are exempt from this rule.

Ownership identification of reflected workloads does not use label injection: in practice the K3k agent overwrites all labels of host Pods on every sync (labels added by the webhook do not survive), while spec.affinity survives (profile affinity injected by the webhook persists). Therefore `VirtualCluster.status.placements` and Cell inventory accounting of VirtualCluster workloads match host Pods by the `k3k.io/clusterName` label (whose value is the resolved K3k cluster name with its 8-character UID suffix, unique per VirtualCluster; the Cell side builds the mapping through the list of VirtualClusters referencing this Cell). The webhook only validates and injects affinity/resources and no longer carries ownership propagation.

K3k native Pods (server/agent: pod-level reservations plus empty containers) are handled by the webhook copying pod-level requests/limits into containers (identified through online diagnosis: LimitRange runs before the webhook and first fills container defaults; empty containers directly inherit them; entries exactly equal to the platform defaults are overwritten from the pod level, while already-declared entries are left untouched). Translation/reflection Pods are not in this category and are still backfilled by LimitRange.

## 6. Boundary Rules

1. The three layers stay independent; they are never merged.
2. One Cell equals one OCM ManagedCluster equals one host cluster; physical nodes are only inventory; each child cluster has exactly one logical child node.
3. The management controller is read-only toward hosts (MSA plus proxy); host writes go only through ManifestWork.
4. No host admin kubeconfig is used; credential bytes and tokens never appear in status/Events/logs.
5. All cleanup is UID-scoped: names embed an 8-character UID prefix, and deletion checks labels and owner UIDs.
6. `runtimeClassName` is never required, defaulted, inferred, translated, or validated by KubeCell; the sole source is the child-cluster workload YAML.
7. Child-cluster hostPath volumes are always rejected with a reason and a replacement; there is no passthrough and no rewriting.
8. Translation from the public child version `v1.34.2+k3s1` to K3k syntax is handled centrally by `provider/profile`; string concatenation is not used. K3k `v1.1.0` is forbidden.
9. Fixed principle: human-authored spec expresses only intent; any field with a single safe value is a platform constant and never enters the CRD; version-class changes go through releases, never through CR edits.
10. `:80`/`:443` have exactly one host-wide binder: host Traefik. No ingress controller may run inside child clusters.

## 7. Upstream Policy and Version Pinning

- Keep a no-fork policy for K3k, K3s, TopoLVM, Multus, Whereabouts, and similar projects; integrate through public APIs, standard Kubernetes resources, adapters, or sidecar/Operator mechanisms.
- Locked combination: host K3s `v1.36.3+k3s1`, K3k `v1.2.0`, child K3s `v1.34.2+k3s1`, OCM Hub `v1.3.1`. The management machine may be x86_64; hosts are ARM64. Versions must never be replaced silently. `v1.2.0` is the formal release (published 2026-08-27, latest stable release), fully including the kubelet parameter passthrough and PVC sync fixes, plus the server-Pod anti-reset-crash and projected-volume token translation fixes; the Shared-mode fields used by KubeCell carry no breaking changes.
- OCM ships with KubeCell but is not owned by KubeCell: neither chart may install/upgrade/delete OCM objects; the installer owns the locked bundle.
- Platform constants (never in CRDs; humans never fill them in): exposure method `NodePort`, port range `30000–32767`, K3k controller namespace `k3k-system`, proxy identity `kubecell-host-reader`, child-cluster server arguments `--disable=traefik` plus `--disable=local-storage` (no usable local-path backend exists inside child clusters; keeping it is only a trap; host-side local-path is separately disabled at installation time, see storage constraints), storage binding `WaitForFirstConsumer`/reclamation `Retain`, reservations (control plane `500m/1Gi`, reflected system `100m/256Mi`), tenant subject always `<child-cluster-name>-admin`, hostPath volumes always rejected, ingress class name always `kubecell`.
- Storage constraints: the provisioner runs only on hosts (TopoLVM); inside child clusters only the mapped StorageClass created by the controller is valid, all other provisioners have no provisioner and their PVCs stay Pending forever (self-evident failure, not intercepted); the K3s built-in local-path (hostPath backend) is disabled at host installation time.

## 8. Ownership and Installation Order

OCM Hub CRDs/controllers/extension components belong to the OCM installer. KubeCell CRDs and the management controller belong to the `kubecell-management` chart. The host webhook and host RBAC belong to the `kubecell-host` chart. The Kite dashboard is built into the `kubecell-management` chart as a conditional dependency (see 8.2) and belongs to neither OCM nor the controllers. K3k, TopoLVM, host Traefik, Device Plugin DaemonSets, and other Kubernetes-side baseline manifests ship at pinned versions with the release bundle (vendored charts/manifests plus digests) and are installed idempotently by the installer. A historical host K3k installation that is not Helm-managed must first step down (delete the controller and CRDs, only when Cluster CRs are already empty after VirtualCluster cleanup) before a fresh install. Kernel drivers, firmware, LVM volume groups, the K3s binaries themselves, and CNI belong to the host platform/GitOps layer, must be healthy beforehand, and are checked — never installed — by `doctor`.

Installation order: apply KubeCell CRDs, install the OCM Hub, wait for OCM readiness, install the management chart, join hosts, manually approve ManagedClusters, install the host chart, install the host baseline (K3k, TopoLVM, Traefik, Device Plugin, pinned versions, idempotent), verify the Cell baseline, create Class/Plan/child clusters.

### 8.1 doctor Checklist (read-only)

Principles: doctor never writes to clusters, installs software, or deletes objects; by default it only runs read-only checks, and `--dry-run` only prints commands without executing them. Any failed item causes a non-zero exit with the item number. The phase column marks whether the item runs before installation (pre), after installation (post), or both.

#### Release-bundle layer (runs locally, no cluster identity needed)

| Number | Check | Content | Pass criteria | Phase |
| --- | --- | --- | --- | --- |
| B1 | Manifest parsing | Parse `compatibility.yaml`, including `appsSuffix` | All fields present; `appsSuffix` is a bare domain (no scheme, no port, lowercase, valid length) | both |
| B2 | Artifact integrity | Every artifact path is inside the bundle, SHA-256 computed one by one | Matches the digests recorded in the manifest item by item | both |
| B3 | Version pinning | K3k and child-K3s versions, controller image references | Equal to the locked combination (see §7); images are immutable digest references | both |

#### doctor management (management kubectl context, identity is the executor's kubeconfig identity, administrator required; all verbs read-only except helm)

| Number | Check | Command and object | Pass criteria | Phase |
| --- | --- | --- | --- | --- |
| M1 | Connectivity | `kubectl version --request-timeout=10s` | Returns the server version successfully | both |
| M2 | Chart renderable | `helm template kubecell-management --namespace kubecell-system` (local render, no cluster writes) | Exit 0 | both |
| M3 | OCM CRDs | `get crd managedclusters.cluster.open-cluster-management.io`, `manifestworks.work.open-cluster-management.io` | Both exist | both |
| M4 | Write permission | `auth can-i create manifestworks.work.open-cluster-management.io --all-namespaces` | `yes` | both |
| M5 | Hub health | `deployment/cluster-manager` (`open-cluster-management`) Available | Available=True | post |

#### doctor host (host kubectl context, identity is the executor's kubeconfig identity, administrator required; all verbs read-only except helm; the DNS item is the exception, see H8)

| Number | Check | Command and object | Pass criteria | Phase |
| --- | --- | --- | --- | --- |
| H1 | Connectivity and rendering | `kubectl version`; `helm template kubecell-host` | Success plus exit 0 | both |
| H2 | K3k | `crd clusters.k3k.io`; `k3k-system` controller Deployment | CRD exists; Deployment Available=True and image tag equals the locked K3k version | both |
| H3 | Nodes and profile | `get nodes` (check Ready and the `hardware.kubecell.io/profile` label); per-node `allocatable` device keys | Everyone Ready; everyone carries one and the same profile value (single-profile constraint); every device key declared by the Cell has positive allocatable (one check covers kernel drivers plus Device Plugin) | both |
| H4 | Storage | StorageClass list; TopoLVM controller/lvmd readiness; node `capacity.topolvm.io/*` annotations | A class with provisioner `topolvm.io` and WaitForFirstConsumer exists; no class with a local-path provisioner exists; lvmd is ready; at least one TopoLVM capacity annotation exists (proxy indicator that the volume group exists; the volume group itself is created by the platform) | both |
| H5 | Ingress uniqueness | Traefik DaemonSet readiness; cluster-wide scan for `hostPort: 80/443` | Ready; nobody except host Traefik binds 80/443 (mechanization of boundary rule 10) | post |
| H6 | KubeCell host components | Host webhook Deployment; MutatingWebhookConfiguration | Present and ready | post |
| H7 | Version consistency | Server K3s version; K3k image tag | Equal to the locked combination | both |
| H8 | DNS (runs locally, no cluster identity, pure DNS query) | Resolve `probe.<appsSuffix>` (for example `probe.apps.example.com`) | Resolves to the host ingress IP | both |

### 8.2 Kite Dashboard (management-plane companion UI)

- Choice: kite-org/kite (open-source multi-cluster dashboard with native CRD support; can display Cell/VirtualCluster directly). At implementation time, pin the official OCI chart version plus digest; do not use unofficial sources.
- Form: Helm dependency of the `kubecell-management` chart (`condition: kite.enabled`, enabled by default); the subchart ships vendored with the release bundle so offline installation works.
- Data: sqlite single-node mode with PVC persistence (size and StorageClass set by values; management-cluster StorageClass availability is an open check before implementation).
- Access: Service uses NodePort for access; the superUser initial credential is injected through values/Secrets and never stored in the repository. The first version only connects to the management cluster; host/child clusters come later.
- RBAC (confirmed): Kite uses a dedicated ServiceAccount bound to a full-privilege ClusterRole (both verbs and resources are `*`). Risk stated explicitly: operations performed through the Kite UI execute with that cluster credential, so the Kite RBAC itself becomes the effective boundary; re-review before production use.
- Images: `ghcr.io/kite-org/kite`, following the image-manual flow; verify the amd64 manifest at installation time.

## 9. Host Onboarding Simplification and CR Usability Design

Goal: compress the manual work for joining a new standalone host to 4 commands; for day-to-day child-cluster creation, a human writes a single 8-line VirtualCluster file; Class creation changes 6 numbers; the Cell is generated by the installer (0 lines hand-written). Complexity belongs to the installer and platform constants, not to controllers and CRDs.

### 9.1 Onboarding Flow (humans run only 4 commands)

1. `doctor` (read-only preflight);
2. `join` (run the locked OCM join command under the host context);
3. `accept` (manually approve the `ManagedCluster` on the management plane; safety gate, retained);
4. `chart` (install the host chart).

Then apply the generated cell/class, then plan, then VirtualCluster. Unchanged: the join/chart split into two steps, manual approval, and controllers never install any host baseline.

### 9.2 Cell Generation

New installer command `host --step=cell`: using the human's dual-cluster administrator identity, it reads the live environment (ManagedCluster name, already-applied profile labels, device keys observed on nodes) and renders Cell YAML; the human only runs `apply`. The naming convention is enforced by the generator: ManagedCluster name equals Cell name. (Addresses and versions are determined platform-side; the generator does not need to read them.)

### 9.3 Baseline Command Templates

`compatibility.yaml` gains host-baseline command templates (following the `OCMJoinCommand` `{{placeholder}}` pattern), covering version-sensitive commands such as K3s installation and label application. Humans copy the commands emitted by the installer and run them without transcribing version numbers. It also gains Kubernetes-side baseline manifests (K3k chart, TopoLVM, Traefik, Device Plugin DaemonSet) with digests, applied idempotently by a new installer step (K3k as a vendored Helm chart; a historical manually installed host K3k steps down first and is then installed fresh). Kernel drivers, volume groups, and other platform prerequisites are only checked, never installed.

### 9.4 Class Draft Generation

New installer command `suggest-class`: derives `workloadHard` from the allocatable capacity observed in `Cell.status.inventory` and outputs a VirtualNodeClass draft; the human only adjusts the numbers down. Device keys are no longer hand-copied and quotas are no longer guessed.

### 9.5 Platform Constants Needing No Hand-Writing

Constants humans never fill in are listed in §7 (versions, port range, controller namespace, proxy identity, server arguments, storage binding/reclamation, reservations, tenant subject, hostPath rejection). The installer and controllers use them directly. Version-drift validation is retained and simplified: controllers compare observed versions against the locked constants and report conditions on mismatch.

### 9.6 Hand-Written Examples (day-to-day form)

For day-to-day child-cluster creation, the human-authored file is exactly these 8 lines (only `cellRef.name` and `classRef.name` are required):

```yaml
apiVersion: kubecell.io/v1alpha1
kind: VirtualCluster
metadata:
  name: child-dev
  namespace: kubecell-system
spec:
  cellRef: {name: cell1}
  classRef: {name: ascend-910b}
```

Class authoring leaves only 6 quota numbers for humans (see the 3.2 example); the Cell is generated by `host --step=cell` (0 lines hand-written). A Plan's `spec.virtualCluster` is isomorphic to a VirtualCluster, so the same file can build a Plan preflight or a VirtualCluster; humans never write it twice.

### 9.7 Explicitly Out of Scope

Controllers never install K3k, K3s, TopoLVM, Device Plugins, or CNI; never auto-approve `ManagedCluster` objects; single profile per Cell (multiple physical machines of the same profile may exist and all count as inventory); machine profiles never become a standalone CRD. The path for adding workers to an existing Cell is untouched (already simple enough; the OCM flow is not modified).

## 10. Current Boundaries (Unverified/Deferred Items)

Single-logical-node constraint: each VirtualCluster has one and only one logical child node (constant name `kubelet`); physical hosts may be plural (same profile, all counted as inventory), but the logical node count is always one. Workloads depending on "one Pod per node" semantics (DaemonSets, topology constraints spread across logical nodes) are not supported. Multiple logical nodes would be a design change and are out of the current scope.

Unverified: multi-host scheduling and TopoLVM affinity inside one Cell, automatic host enrollment, multi-replica controllers, release-bundle online install/upgrade/rollback/uninstall drills, Ingress endpoint synchronization and reflected-Pod IP routability, and the hostPath rejection message wording. Deferred: multi-Cell isolation, full version certification, advanced accelerator features (MIG/time-slicing/sharing), standalone workload placement, full controller lifecycle certification, and non-HTTP north-south traffic. The task list is tracked in `TODOS.md`; the current CI plan is in `docs/testing/ci-plan.md`.

## 11. External Exposure and Ingress (North-South)

### 11.1 Single-Ingress Decision

Host `:80`/`:443` may have exactly one binder per host machine; N child clusters each installing an ingress controller and exclusively occupying port 80 cannot hold simultaneously. Decision: the host Traefik DaemonSet (HostPort 80/443) is the single host-wide north-south ingress; no ingress controller may run inside child clusters (child-cluster Traefik stays disabled; user-installed ones are equally unsupported: they would race for ports and have no route). Developers keep writing standard Ingress objects (portable); the implementation is fixed to host Traefik — standard API, fixed implementation.

### 11.2 Routing Model and Naming Rules

Routing is by Host header. A complete hostname allows exactly one shape: `<ingress-name>.<vc-name>.<fixed-suffix>` (for example `web.team1.apps.example.com`). Developers only choose the Ingress name (first segment, must be DNS-safe); the middle segment is always the owning VirtualCluster name; the fixed suffix is a global configuration with global effect: it lives in the release bundle's `compatibility.yaml` as `appsSuffix` (for example `apps.example.com`), and the installer passes it to the management controller as a Helm value. The controller validates the suffix at startup (bare domain) and refuses to start when invalid. A single global value is safe only because VirtualCluster names are globally unique (already enforced by the webhook). Changing the suffix requires four things at once: the DNS wildcard, the wildcard certificate, a controller re-release, and re-rendering of all host Ingresses. With multiple Cells a single wildcard cannot point at N different host IPs, see 11.7.

### 11.3 Endpoint Synchronization Mechanism

The controller polls Ingresses with `ingressClassName=kubecell` inside child clusters, mirrors host Services and Endpoints inside the VirtualCluster's host namespace, and renders a same-named host Ingress (class `kubecell`) pointing at them. Mirrored objects ship with the instance Work; host writes go only through ManifestWork and the hard rule does not move. Reflected Pod IPs equal child Pod IPs (allocated by the host CNI, routable on hosts; confirmed through online verification), and host Endpoints reference only their address subset with port names always blanked (when an Ingress backend is specified numerically, Traefik matches endpoints by port name; keeping a name misses and yields 503; identified through online diagnosis). Host Traefik only watches VirtualCluster namespaces carrying the KubeCell label.

Execution layering: naming rules are guaranteed by KubeCell at assembly time — the `host` field of child-cluster Ingresses is always ignored, and the host-side hostname is always assembled per 11.2, so illegal domains have no input path and cannot occur. Traefik only cooperates (namespace label selection plus ingress-class filtering) and performs no hostname validation. The host-side render format uses standard Ingress in v1; IngressRoute is adopted later only when per-path/per-header advanced matching is needed, without changing ports or adding components. Gateway API (whose listener wildcards carry hostname-intersection semantics) is recorded as a long-term path: evaluate it later if per-VirtualCluster subdomain autonomy is ever needed.

### 11.4 TLS

Hosts terminate TLS uniformly; certificates come from platform-provided wildcard certificates. The default is HTTP; TLS is optional (certificate sources are platform-provided, never self-issued automatically).

### 11.5 Scope

v1 supports only HTTP(S); non-HTTP traffic such as TCP/UDP is not yet supported; child-cluster Traefik stays disabled.

### 11.6 Installation Ownership

The Traefik DaemonSet, wildcard DNS, and certificates are all host baseline, installed by the installer at pinned versions (see §8). Controllers only synchronize Ingress and endpoint objects and never install controllers.

### 11.7 DNS Automation (enabled when the second host arrives)

In the single-host phase one manual wildcard record is sufficient (zero management with wildcard DNS). Trigger: the second host/second Cell comes online — a single wildcard record cannot point at N different host IPs.

Plan: one external-dns per host (pinned version, installed by the installer): it watches only host Ingresses with `class=kubecell` and creates per-record A records through the DNS provider API; the domain filter is pinned to the fixed suffix; `--txt-owner-id=<cell-name>` establishes ownership; the target IP takes that Cell's static host IP value (written by the installer, never depending on Traefik status writeback). The provider API key is a platform Secret, installed by the installer and never touched by controllers. The same credential can later serve DNS-01 wildcard certificates.

Alternative: when the DNS provider has no API, split the suffix per Cell (`<ingress-name>.<vc-name>.<cell-name>.<fixed-suffix>` plus one wildcard per Cell), at the cost of changing the naming rule and leaking topology. A single VIP/keepalived is rejected: it cannot solve pointing at the right host.

## 12. User Stories

### US-1 Developer Creates a Child Cluster and Obtains kubeconfig

As a developer, I want to create a VirtualCluster under some Cell and obtain a working admin kubeconfig.

Prerequisites: the target Cell is Ready (OCM joined with a fresh Lease, fresh inventory); the required quota-tier VirtualNodeClass already exists.

Flow:

1. Write an 8-line VirtualCluster file, filling in only the target Cell name and quota-tier name (see 9.6).
2. (Recommended) First create a VirtualClusterPlan with the same content and wait for `status.decision=Accepted` before continuing; see US-2.
3. `kubectl apply -f vc.yaml`; the webhook validates that references exist and quota keys are legal.
4. The controller resolves the Cell and quota tier and reruns preflight before creation; a non-Accepted conclusion stops the flow, with the check report written to `status.feasibilityChecks`.
5. The controller creates the foundation Work (host namespace, quota, policy) and, once it is Applied, creates the instance Work (K3k cluster and policy).
6. The OCM work-agent lands on the host and executes; K3k creates the server Pod and data volume, NodePort Service, and child-cluster API.
7. The controller reads the K3k-generated kubeconfig Secret through the proxy, rewrites the server address to the auto-discovered host address and NodePort, and publishes it as a management-plane Secret.
8. The VirtualCluster enters `phase=Ready`.
9. The user fetches the Secret from `status.credential.secretName`, exports a kubeconfig file (mode 600), and verifies with `kubectl --kubeconfig`: the single logical node `kubelet` is visible.

Acceptance: `phase=Ready`; the kubeconfig lists the logical node; the child cluster can run Pods, PVCs, and Services (see the test plan).

### US-2 Seeing Inventory and Feasibility Before Creation

As a developer, before really creating a child cluster, I want to see the resource inventory of the Cell and know whether this creation can succeed.

Prerequisite: the target Cell object exists (Ready is not required; otherwise there is no inventory to inspect).

Flow:

1. `kubectl get cell <name> -o yaml` and inspect `status`: `nodes` (per-node capacity, allocatable, used, pending), `inventory` (aggregated headroom), `storageInventory` (whether TopoLVM free space is known), `conditions` (whether inventory is fresh).
2. If `InventoryFresh` is false or key inventory is unknown, stop and contact operations: no conclusion is trustworthy at this point; do not create.
3. Write a VirtualClusterPlan file (isomorphic to a VirtualCluster), `kubectl apply`.
4. Wait for `status.decision`: `Accepted` means buildable under the current snapshot; `Rejected` means reading the failed items in `checks` (missing CPU, missing cards, missing storage, missing ports) and adjusting the request or switching Cells; `Unknown` means insufficient information, retry once inventory is fresh.
5. Note `expiresAt`: conclusions expire and are void; re-applying the Plan yields a fresh evaluation (Plans evaluate once and never refresh periodically). Acceptance never equals reservation — the controller rechecks at real creation time; if capacity was consumed by others in between, creation is blocked and the recheck report appears in `status.feasibilityChecks`.
6. After passing, convert the same file into a VirtualCluster creation and enter US-1.

Acceptance: the Plan yields exactly one of the three states; a `Rejected` conclusion carries failure reasons that guide request changes; expired conclusions are never adopted.

### US-3 Operations Onboards Physical Nodes

As an operations administrator, I want to onboard physical nodes into the KubeCell system quickly. Two cases:

Case A: first node of a new standalone Cell (full onboarding):

1. Prepare the host baseline from the release-bundle templates: platform-side prerequisites (K3s, CNI, kernel drivers and firmware, LVM volume groups, profile labels, preloaded images) must be ready beforehand; Kubernetes-side manifests (K3k, TopoLVM, Traefik, Device Plugin) are installed by the installer at pinned versions.
2. Run 4 commands: `doctor` (read-only preflight), `join` (run the locked OCM onboarding command on the host), `accept` (manually approve the `ManagedCluster` on the management plane; safety gate), `chart` (install the host chart).
3. The installer installs the host baseline (K3k, TopoLVM, Traefik, Device Plugin, pinned versions, idempotent), then `host --step=cell` generates the Cell from the live environment, `kubectl apply`.
4. Confirm the Cell is Ready: joined, reachable, fresh Lease, fresh inventory.

Case B: appending same-profile workers to an existing Cell:

1. Join the existing host cluster as a K3s agent (with an independent data directory; deletes no data on that machine). No OCM onboarding is run and no new Cell is created.
2. Apply profile labels and wait for node Ready and platform DaemonSets (Device Plugin, TopoLVM node components) to become ready.
3. Confirm the new node appears in `Cell.status.nodes` and aggregated inventory increases.

Acceptance: the Cell is Ready; `status.nodes` matches the new machines; workloads of newly created test child clusters can be scheduled. In case B, existing workloads are neither migrated nor interrupted.

## 13. Glossary

| Term | Meaning |
| --- | --- |
| Management cluster | Single-node K3s carrying the OCM Hub and the KubeCell management controller; the sole source of desired state and status observation; runs no business workloads |
| OCM ManagedCluster | Registration object in the management-cluster OCM Hub, representing one Cell host cluster one-to-one; not a standalone cluster |
| Cell | One OCM `ManagedCluster` plus its host K3s cluster; failure/isolation boundary |
| VirtualNodeClass | Cluster-scoped quota tier: quota hard-limit numbers plus storage name |
| VirtualCluster | Child-cluster instance (the product itself), key fields immutable, rich status |
| VirtualClusterPlan | Side-effect-free preflight (one-shot evaluation); acceptance never equals reservation |
| Logical child node | The single node in the child-cluster API (constant name `kubelet`); view only, no CRD, observations folded into VirtualCluster status |
| Foundation Work | Host namespace plus policy labels/annotations, ResourceQuota, LimitRange (container default 100m/256Mi), NetworkPolicy, portforward RBAC |
| Instance Work | K3k `Cluster` (Shared mode) plus K3k VirtualClusterPolicy |
| Resolution snapshot | `VirtualCluster.status.resolved`, the sole basis for steady-state rendering |
| Reflected Pod | Host Pod created by K3k mirroring a child-cluster Pod, with identical resource requests |
| Cluster Proxy / MSA | OCM mechanisms; the only path by which the management controller reads host APIs |
| Host Traefik | The single host-wide north-south ingress (HostPort 80/443); watches only VirtualCluster namespaces carrying the KubeCell label |
| Fixed suffix (appsSuffix) | Platform-global external domain suffix; child-cluster external domains derive as `<ingress-name>.<vc-name>.<fixed-suffix>`; developers only choose the Ingress name |
