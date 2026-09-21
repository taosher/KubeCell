import { Callout, Prose } from "@/components/docs/callout";
import { CodeBlock } from "@/components/code-block";
import { Highlighter } from "@/components/magicui/highlighter";
import { buildMetadata } from "@/lib/metadata";

export const metadata = buildMetadata({
  title: "API reference",
  description:
    "KubeCell CRD reference for kubecell.io/v1alpha1: Cell, VirtualNodeClass, VirtualClusterPlan, and VirtualCluster spec and status fields, immutability rules, and phases.",
  path: "/docs/api-reference/",
  keywords: [
    "KubeCell API reference",
    "kubecell.io/v1alpha1",
    "Cell CRD",
    "VirtualCluster CRD fields",
    "VirtualNodeClass spec",
  ],
});

function FieldTable({
  rows,
}: {
  rows: Array<[string, string, string, string]>;
}) {
  return (
    <div className="not-prose my-6 overflow-hidden rounded-xl border border-white/10">
      <div className="overflow-x-auto">
        <table className="w-full min-w-[720px] border-collapse text-left text-sm">
          <thead>
            <tr className="bg-white/[0.03]">
              <th className="px-4 py-3 font-medium text-muted-foreground">Field</th>
              <th className="px-4 py-3 font-medium text-muted-foreground">Type</th>
              <th className="px-4 py-3 font-medium text-muted-foreground">Required</th>
              <th className="px-4 py-3 font-medium text-muted-foreground">Notes</th>
            </tr>
          </thead>
          <tbody className="text-muted-foreground">
            {rows.map(([field, type, required, notes]) => (
              <tr key={field} className="border-t border-white/5">
                <td className="px-4 py-3 font-mono text-xs text-sky-300">{field}</td>
                <td className="px-4 py-3 font-mono text-xs">{type}</td>
                <td className="px-4 py-3">{required}</td>
                <td className="px-4 py-3">{notes}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

export default function ApiReferencePage() {
  return (
    <Prose>
      <p className="font-mono text-xs uppercase tracking-[0.2em] text-sky-400">Guides</p>
      <h1 className="mt-3 text-3xl font-semibold tracking-tight text-foreground sm:text-4xl">
        API reference
      </h1>
      <p className="mt-5 text-base leading-7 text-muted-foreground">
        KubeCell owns exactly four CRDs under <code>kubecell.io/v1alpha1</code>. All of them are
        desired-state or preflight APIs. Fields with a single safe value are platform constants and
        intentionally absent.
      </p>

      <div className="not-prose my-6 overflow-hidden rounded-xl border border-white/10">
        <table className="w-full border-collapse text-left text-sm">
          <thead>
            <tr className="bg-white/[0.03]">
              <th className="px-4 py-3 font-medium text-muted-foreground">Kind</th>
              <th className="px-4 py-3 font-medium text-muted-foreground">Scope</th>
              <th className="px-4 py-3 font-medium text-muted-foreground">Short name</th>
              <th className="px-4 py-3 font-medium text-muted-foreground">Owner</th>
            </tr>
          </thead>
          <tbody className="text-muted-foreground">
            <tr className="border-t border-white/5">
              <td className="px-4 py-3 font-mono text-xs text-sky-300">Cell</td>
              <td className="px-4 py-3">Namespaced</td>
              <td className="px-4 py-3 font-mono text-xs">cell</td>
              <td className="px-4 py-3">Platform</td>
            </tr>
            <tr className="border-t border-white/5">
              <td className="px-4 py-3 font-mono text-xs text-sky-300">VirtualNodeClass</td>
              <td className="px-4 py-3">Cluster-scoped</td>
              <td className="px-4 py-3 font-mono text-xs">vnclass</td>
              <td className="px-4 py-3">Platform</td>
            </tr>
            <tr className="border-t border-white/5">
              <td className="px-4 py-3 font-mono text-xs text-sky-300">VirtualClusterPlan</td>
              <td className="px-4 py-3">Namespaced</td>
              <td className="px-4 py-3 font-mono text-xs">vcp</td>
              <td className="px-4 py-3">Tenant / platform</td>
            </tr>
            <tr className="border-t border-white/5">
              <td className="px-4 py-3 font-mono text-xs text-sky-300">VirtualCluster</td>
              <td className="px-4 py-3">Namespaced</td>
              <td className="px-4 py-3 font-mono text-xs">vc</td>
              <td className="px-4 py-3">Tenant / platform</td>
            </tr>
          </tbody>
        </table>
      </div>

      <Callout variant="info" title="Namespace">
        Cells must live in the same namespace as the controller, by convention{" "}
        <code>kubecell-system</code>.
      </Callout>

      <h2 id="cell">Cell</h2>
      <p>
        The unit of physical capacity: one OCM <code>ManagedCluster</code> bound to one machine
        profile.
      </p>
      <h3>spec</h3>
      <FieldTable
        rows={[
          [
            "managedClusterRef.name",
            "string",
            "Yes",
            "Bound OCM ManagedCluster name. Immutable once any VirtualCluster resolves the Cell.",
          ],
          [
            "machineProfile.name",
            "string",
            "Yes",
            "Profile name; the placement selector. Immutable once resolved.",
          ],
          [
            "machineProfile.devices[]",
            "array",
            "No",
            "Inline accelerator contract. A single machine profile per Cell, not a list of profiles.",
          ],
          [
            "machineProfile.devices[].name",
            "string",
            "Yes",
            "Stable identifier inside KubeCell, referenced by entitlement checks.",
          ],
          [
            "machineProfile.devices[].resourceName",
            "string",
            "Yes",
            "Exact host extended-resource key; must match character-for-character everywhere.",
          ],
        ]}
      />
      <h3>status (read-only)</h3>
      <FieldTable
        rows={[
          ["observedGeneration", "int64", "—", "Observed metadata.generation; distinguishes stale from fresh."],
          [
            "phase",
            "enum",
            "—",
            "Pending | Provisioning | Ready | Degraded | Deleting | Failed.",
          ],
          ["conditions[]", "[]Condition", "—", "Joined, Available, ProviderReady, InventoryFresh, CapabilityMismatch, and others."],
          [
            "managedCluster{name,uid,joined,available,leaseFresh,leaseRenewTime}",
            "object",
            "—",
            "OCM observation plus the decisive Lease freshness signal.",
          ],
          [
            "provider{k3kVersion,namespaceReady,k3kCRDsReady,controllerReady,cniReady,topolvmReady}",
            "object",
            "—",
            "Decomposition of host-baseline readiness.",
          ],
          [
            "nodes[]{name,profile,ready,labelHash,capacity,allocatable,activeRequested,pendingRequested,readinessReasons,observedAt}",
            "array",
            "—",
            "Per-node inventory and placement inputs.",
          ],
          [
            "inventory{<resource>{capacity,allocatable,requested,pending,availableEstimate}}",
            "map",
            "—",
            "Aggregated headroom per resource.",
          ],
          [
            "storageInventory{provisioner,allocatedPV,perNode,freeCapacity,freeCapacityKnown,freeCapacitySource,fresh,observedAt}",
            "object",
            "—",
            "TopoLVM free space must come from lvmd; inferring from PV capacity is forbidden.",
          ],
          ["nodePorts{allocated,observedAt,fresh}", "object", "—", "Allocated NodePorts for child APIs."],
        ]}
      />
      <CodeBlock
        className="not-prose my-6"
        language="yaml"
        filename="cell-full.yaml"
        code={`apiVersion: kubecell.io/v1alpha1
kind: Cell
metadata:
  name: cell1
  namespace: kubecell-system
spec:
  managedClusterRef:
    name: cell1
  machineProfile:
    name: ascend-910b
    devices:
    - name: ascend
      resourceName: huawei.com/Ascend910
status:
  observedGeneration: 1
  phase: Ready
  managedCluster:
    name: cell1
    uid: "b3f1..."        # compared at UID level on deletion
    joined: true
    available: true       # not trustworthy alone
    leaseFresh: true      # the true liveness criterion
  provider:
    k3kVersion: v1.2.0
    namespaceReady: true
    k3kCRDsReady: true
    controllerReady: true
    cniReady: true
    topolvmReady: true`}
      />

      <h2 id="virtualnodeclass">VirtualNodeClass</h2>
      <p>Cluster-scoped quota tier: one name, one entitlement, one storage class.</p>
      <FieldTable
        rows={[
          [
            "entitlement.workloadHard",
            "map[string]Quantity",
            "Yes",
            "Hard requests/limits applied as a ResourceQuota in the host namespace.",
          ],
          [
            "storageClassName",
            "string",
            "Yes",
            "Storage class used for child data volumes and child PVCs (typically topolvm-provisioner).",
          ],
        ]}
      />
      <CodeBlock
        className="not-prose my-6"
        language="yaml"
        filename="virtualnodeclass-full.yaml"
        code={`apiVersion: kubecell.io/v1alpha1
kind: VirtualNodeClass
metadata:
  name: ascend-910b-small
spec:
  entitlement:
    workloadHard:
      requests.cpu: "4"
      limits.cpu: "4"
      requests.memory: 8Gi
      limits.memory: 8Gi
      requests.huawei.com/Ascend910: "2"
      limits.huawei.com/Ascend910: "2"
  storageClassName: topolvm-provisioner`}
      />

      <h2 id="virtualclusterplan">VirtualClusterPlan</h2>
      <p>
        Side-effect-free preflight. The spec is isomorphic to a VirtualCluster so an accepted plan can
        be turned into a creation request without rewriting anything.
      </p>
      <FieldTable
        rows={[
          ["cellRef.name", "string", "Yes", "Target Cell."],
          ["classRef.name", "string", "Yes", "Target quota tier."],
          [
            "status.decision",
            "enum",
            "—",
            "Accepted | Rejected | Unknown. Evaluated once; never refreshed periodically.",
          ],
          ["status.expiresAt", "time", "—", "When the conclusion becomes void."],
          ["status.checks[]", "array", "—", "Per-resource findings, including the reason for Rejected."],
        ]}
      />
      <Callout variant="warning" title="Not a reservation">
        <Highlighter color="#38bdf8" strokeWidth={1.5} isView>
          Acceptance never reserves capacity.
        </Highlighter>{" "}
        The controller re-runs the same evaluation at real creation time and blocks on a non-accepted
        conclusion, writing the report to <code>VirtualCluster.status.feasibilityChecks</code>.
      </Callout>

      <h2 id="virtualcluster">VirtualCluster</h2>
      <p>The child cluster instance: key fields immutable, rich status.</p>
      <h3>spec</h3>
      <FieldTable
        rows={[
          ["cellRef.name", "string", "Yes", "Target Cell; immutable after creation."],
          ["classRef.name", "string", "Yes", "Quota tier; immutable after creation."],
        ]}
      />
      <h3>status (read-only)</h3>
      <FieldTable
        rows={[
          ["observedGeneration", "int64", "—", "Generation the status reflects."],
          [
            "phase",
            "enum",
            "—",
            "Pending | Provisioning | Ready | Degraded | Deleting | Failed.",
          ],
          [
            "resolved",
            "object",
            "—",
            "Immutable resolution snapshot: the sole basis for steady-state rendering.",
          ],
          [
            "feasibilityChecks[]",
            "array",
            "—",
            "Recheck report written at creation time.",
          ],
          ["endpoint{address,port}", "object", "—", "Discovered child API address and NodePort."],
          [
            "credential.secretName",
            "string",
            "—",
            "Secret containing the admin kubeconfig (kubeconfig.yaml key).",
          ],
          ["host.quotaUsed", "object", "—", "Quota accounting observed on the host side."],
          ["conditions[]", "[]Condition", "—", "Standard condition table."],
        ]}
      />
      <CodeBlock
        className="not-prose my-6"
        language="yaml"
        filename="virtualcluster-status.yaml"
        code={`apiVersion: kubecell.io/v1alpha1
kind: VirtualCluster
metadata:
  name: child-dev
  namespace: kubecell-system
spec:
  cellRef: {name: cell1}
  classRef: {name: ascend-910b-small}
status:
  phase: Ready
  endpoint:
    address: 10.0.12.7
    port: 31234
  credential:
    secretName: kubecell-child-dev-<uid>-admin-kubeconfig
  host:
    quotaUsed:
      requests.cpu: "400m"
      requests.huawei.com/Ascend910: "2"`}
      />

      <h2 id="phases">Phases and conditions</h2>
      <div className="not-prose my-6 overflow-hidden rounded-xl border border-white/10">
        <table className="w-full border-collapse text-left text-sm">
          <thead>
            <tr className="bg-white/[0.03]">
              <th className="px-4 py-3 font-medium text-muted-foreground">Phase</th>
              <th className="px-4 py-3 font-medium text-muted-foreground">Meaning</th>
            </tr>
          </thead>
          <tbody className="text-muted-foreground">
            {[
              ["Pending", "Accepted, waiting for the controller to begin rendering."],
              ["Provisioning", "Works are being applied on the host; child cluster not yet Ready."],
              ["Ready", "Child API reachable, kubeconfig published, quota and policy in place."],
              ["Degraded", "The Cell lost liveness (stale lease) or a required inventory source is unavailable."],
              ["Deleting", "Finalizer cleanup in progress by UID."],
              ["Failed", "A terminal error that requires operator action; re-applying does not bypass it."],
            ].map(([phase, meaning]) => (
              <tr key={phase} className="border-t border-white/5">
                <td className="px-4 py-3 font-mono text-xs text-sky-300">{phase}</td>
                <td className="px-4 py-3">{meaning}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <ul>
        <li>
          <strong>Joined / Available</strong> — OCM conditions. Available alone is not trustworthy.
        </li>
        <li>
          <strong>ProviderReady</strong> — host baseline decomposed into K3k, CNI, and TopoLVM
          readiness.
        </li>
        <li>
          <strong>InventoryFresh</strong> — inventory and storage observations are recent and from a
          live source.
        </li>
        <li>
          <strong>CapabilityMismatch</strong> — declared device capacity exceeds observed
          allocatable.
        </li>
      </ul>

      <p>
        See also: <a href="https://github.com/taosher/KubeCell/blob/main/technical-design.md">technical-design.md</a>{" "}
        for the authoritative field-by-field design and validation rules.
      </p>
    </Prose>
  );
}
