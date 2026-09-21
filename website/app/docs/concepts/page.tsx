import Link from "next/link";

import { Callout, Prose } from "@/components/docs/callout";
import { CodeBlock } from "@/components/code-block";
import { buildMetadata } from "@/lib/metadata";

export const metadata = buildMetadata({
  title: "Core concepts",
  description:
    "KubeCell core concepts explained: the four resources you work with, quota tiers, how a cluster maps to a host, and what happens when you create, resize, or delete one.",
  path: "/docs/concepts/",
  keywords: [
    "KubeCell Cell",
    "VirtualNodeClass",
    "VirtualClusterPlan",
    "logical node kubelet",
    "reflected Pod",
    "resolution snapshot",
  ],
});

const cellExample = `apiVersion: kubecell.io/v1alpha1
kind: Cell
metadata:
  name: cell1
  namespace: kubecell-system
spec:
  managedClusterRef:
    name: cell1              # bound OCM ManagedCluster; immutable once resolved
  machineProfile:
    name: ascend-910b        # single machine profile per Cell
    devices:
    - name: ascend
      resourceName: huawei.com/Ascend910  # exact host extended-resource key`;

const classExample = `apiVersion: kubecell.io/v1alpha1
kind: VirtualNodeClass
metadata:
  name: ascend-910b-small    # cluster-scoped; the tier name tenants reference
spec:
  entitlement:
    workloadHard:
      requests.cpu: "4"
      limits.cpu: "4"
      requests.memory: 8Gi
      limits.memory: 8Gi
      requests.huawei.com/Ascend910: "2"
      limits.huawei.com/Ascend910: "2"
  storageClassName: topolvm-provisioner`;

const planExample = `apiVersion: kubecell.io/v1alpha1
kind: VirtualClusterPlan
metadata:
  name: child-dev-plan
  namespace: kubecell-system
spec:
  cellRef: {name: cell1}
  classRef: {name: ascend-910b-small}
status:
  decision: Accepted          # Accepted | Rejected | Unknown
  expiresAt: "2026-09-22T10:15:00Z"   # stale conclusions are void
  checks: []                  # per-resource findings when Rejected`;

export default function ConceptsPage() {
  return (
    <Prose>
      <p className="font-mono text-xs uppercase tracking-[0.2em] text-sky-400">Introduction</p>
      <h1 className="mt-3 text-3xl font-semibold tracking-tight text-foreground sm:text-4xl">
        Core concepts
      </h1>
      <p className="mt-5 text-base leading-7 text-muted-foreground">
        Six ideas carry the whole design. If you understand these, the CRDs and the operational
        behavior will feel predictable.
      </p>

      <h2 id="cell">Cell</h2>
      <p>
        A <strong>Cell</strong> is the unit of physical capacity: one OCM <code>ManagedCluster</code>{" "}
        bound to one host cluster, plus a single machine profile describing what the machines offer.
        It is the failure and isolation boundary. The spec has exactly two groups:
      </p>
      <ul>
        <li>
          <code>managedClusterRef.name</code> — the bound OCM <code>ManagedCluster</code>. Once any
          VirtualCluster has resolved this Cell, the reference is immutable.
        </li>
        <li>
          <code>machineProfile</code> — a name and an inline device contract. Multiple physical
          machines with the same profile all count as inventory; there is no separate{" "}
          <code>MachineProfile</code> CRD.
        </li>
      </ul>
      <p>
        Everything else lives in <code>status</code>, filled in by the controller: phase, conditions,
        managed cluster observations, provider readiness, per-node capacity and allocatable,
        aggregated inventory, storage inventory, and allocated NodePorts. Never hand-write status.
      </p>
      <CodeBlock className="not-prose my-6" code={cellExample} language="yaml" filename="cell.yaml" />
      <Callout variant="info" title="Device contract, not reservation">
        The device list is a declarative authorization. The observed allocatable of the resource key
        is the source of fact. If a Cell declares eight cards but the host only allocates four, the
        effective capacity is four and a <code>CapabilityMismatch</code> condition is reported.
      </Callout>

      <h2 id="virtualnodeclass">VirtualNodeClass</h2>
      <p>
        A <strong>VirtualNodeClass</strong> is a cluster-scoped quota tier. One name maps to one
        entitlement: the hard requests and limits applied to every child cluster that uses it, plus
        the storage class for child volumes.
      </p>
      <ul>
        <li>
          The class is applied as a Kubernetes <code>ResourceQuota</code> in the child cluster&apos;s
          host namespace, so enforcement happens on the host, not in the child cluster.
        </li>
        <li>
          CPU, memory, and extended resources (accelerators) all follow the same pattern: request
          equals limit, integer values for devices.
        </li>
        <li>
          Classes are platform-owned. Tenants reference them but never edit them; changing a tier
          means changing the class, which is a platform action.
        </li>
      </ul>
      <CodeBlock
        className="not-prose my-6"
        code={classExample}
        language="yaml"
        filename="virtualnodeclass.yaml"
      />

      <h2 id="virtualclusterplan">VirtualClusterPlan</h2>
      <p>
        A <strong>VirtualClusterPlan</strong> is a side-effect-free preflight. Its spec is isomorphic
        to a VirtualCluster, so you can copy a plan file into a creation request once it is accepted.
        The controller evaluates the request against the current Cell inventory and reports exactly
        one decision:
      </p>
      <div className="not-prose my-6 overflow-hidden rounded-xl border border-white/10">
        <table className="w-full border-collapse text-left text-sm">
          <thead>
            <tr className="bg-white/[0.03]">
              <th className="px-4 py-3 font-medium text-muted-foreground">Decision</th>
              <th className="px-4 py-3 font-medium text-muted-foreground">Meaning</th>
              <th className="px-4 py-3 font-medium text-muted-foreground">What to do</th>
            </tr>
          </thead>
          <tbody className="text-muted-foreground">
            <tr className="border-t border-white/5">
              <td className="px-4 py-3 font-mono text-xs text-emerald-300">Accepted</td>
              <td className="px-4 py-3">Buildable under the current inventory snapshot.</td>
              <td className="px-4 py-3">Create the VirtualCluster (rechecked at creation time).</td>
            </tr>
            <tr className="border-t border-white/5">
              <td className="px-4 py-3 font-mono text-xs text-rose-300">Rejected</td>
              <td className="px-4 py-3">
                A required resource is missing: CPU, memory, devices, storage, or ports.
              </td>
              <td className="px-4 py-3">Read <code>checks</code> and adjust the request or Cell.</td>
            </tr>
            <tr className="border-t border-white/5">
              <td className="px-4 py-3 font-mono text-xs text-amber-300">Unknown</td>
              <td className="px-4 py-3">Inventory is unavailable or stale.</td>
              <td className="px-4 py-3">Fix freshness, then retry once observations recover.</td>
            </tr>
          </tbody>
        </table>
      </div>
      <Callout variant="warning" title="Acceptance is not reservation">
        Plans evaluate once and never refresh periodically. <code>expiresAt</code> marks when the
        conclusion becomes void, and the controller re-runs feasibility at real creation time. If
        someone else consumed capacity in between, creation is blocked and the recheck report appears
        in <code>status.feasibilityChecks</code>.
      </Callout>
      <CodeBlock className="not-prose my-6" code={planExample} language="yaml" filename="plan.yaml" />

      <h2 id="virtualcluster">VirtualCluster</h2>
      <p>
        A <strong>VirtualCluster</strong> is the product. Eight lines of spec: which Cell, which
        class. The controller does the rest:
      </p>
      <ol>
        <li>Resolves the Cell and class, then writes an immutable resolution snapshot.</li>
        <li>Re-runs feasibility and stops on a non-accepted conclusion.</li>
        <li>Creates the Foundation Work, then the Instance Work once the first is Applied.</li>
        <li>Discovers the child API address from host nodes and publishes the admin kubeconfig.</li>
        <li>Reports readiness, quota usage, and per-reflected-Pod observations in status.</li>
      </ol>
      <p>
        Key fields are immutable: the Cell reference and class reference cannot be silently changed.
        To move a workload to different hardware, create a new child cluster. To change the size of a
        child cluster, switch to a different class.
      </p>

      <h2 id="logical-node">The logical node</h2>
      <p>
        Every child cluster exposes exactly one node, always named <code>kubelet</code>. It is an
        entitlement view, not a physical machine. This is why there are no horizontal fields — no node
        count, node pools, or replicas — and why DaemonSet-style semantics are unsupported.
      </p>
      <CodeBlock
        className="not-prose my-6"
        language="bash"
        filename="nodes.sh"
        code={`$ kubectl --kubeconfig child-dev.yaml get nodes
NAME      STATUS   ROLES    AGE   VERSION
kubelet   Ready    <none>   42s   v1.34.2+k3s1`}
      />

      <h2 id="reflected-pod">Reflected Pods and PVCs</h2>
      <p>
        K3k mirrors child-cluster Pods and PVCs into the child&apos;s host namespace. A{" "}
        <strong>reflected Pod</strong> is a real host Pod with identical resource requests, so the
        host scheduler places it and the namespace quota accounts for it. The same is true for
        storage: a child PVC becomes a host PVC backed by TopoLVM.
      </p>
      <ul>
        <li>
          Scheduling decisions stay in Kubernetes — there is no second scheduler to keep in sync.
        </li>
        <li>
          <code>hostPath</code> volumes are rejected at admission with a replacement hint pointing to
          the storage class.
        </li>
        <li>
          Accelerator requests must use the identical extended-resource key in the child and on the
          host.
        </li>
      </ul>

      <h2 id="works">Foundation Work and Instance Work</h2>
      <p>
        The controller renders exactly two OCM <code>ManifestWork</code> objects per child cluster.
        The split is intentional: guardrails first, workload second.
      </p>
      <div className="not-prose my-6 grid gap-4 sm:grid-cols-2">
        <div className="rounded-xl border border-white/10 bg-white/[0.02] p-5">
          <p className="text-sm font-semibold text-foreground">Foundation Work</p>
          <ul className="mt-3 space-y-2 text-sm leading-6 text-muted-foreground">
            <li>Host namespace with policy labels and annotations</li>
            <li>
              <code>ResourceQuota</code> from the class entitlement
            </li>
            <li>
              <code>LimitRange</code> with 100m CPU / 256Mi memory defaults
            </li>
            <li>
              <code>NetworkPolicy</code> denying cross-namespace traffic
            </li>
            <li>Port-forward RBAC for the child cluster</li>
          </ul>
        </div>
        <div className="rounded-xl border border-white/10 bg-white/[0.02] p-5">
          <p className="text-sm font-semibold text-foreground">Instance Work</p>
          <ul className="mt-3 space-y-2 text-sm leading-6 text-muted-foreground">
            <li>
              K3k <code>Cluster</code> in Shared mode
            </li>
            <li>K3k VirtualClusterPolicy</li>
            <li>Server Pod, data volume, NodePort Service created by K3k</li>
            <li>Child admin kubeconfig Secret read back through the proxy</li>
          </ul>
        </div>
      </div>

      <h2 id="snapshot">Resolution snapshot</h2>
      <p>
        <code>VirtualCluster.status.resolved</code> is the sole basis for steady-state rendering. It
        records the resolved Cell, class entitlement, storage class, and the platform constants that
        applied at resolution time. If a class changes later, existing child clusters keep the
        snapshot they were built from until the operator acts — that is how KubeCell avoids
        surprise in-place mutations.
      </p>

      <h2 id="constants">Platform constants vs. API fields</h2>
      <p>
        A field belongs in a CRD only if it has more than one safe value. Everything else is a
        platform constant owned by the release:
      </p>
      <div className="not-prose my-6 overflow-hidden rounded-xl border border-white/10">
        <table className="w-full border-collapse text-left text-sm">
          <thead>
            <tr className="bg-white/[0.03]">
              <th className="px-4 py-3 font-medium text-muted-foreground">Concern</th>
              <th className="px-4 py-3 font-medium text-muted-foreground">Where it lives</th>
            </tr>
          </thead>
          <tbody className="text-muted-foreground">
            <tr className="border-t border-white/5">
              <td className="px-4 py-3">K3s, K3k, OCM versions</td>
              <td className="px-4 py-3">Release bundle, verified by the installer</td>
            </tr>
            <tr className="border-t border-white/5">
              <td className="px-4 py-3">API exposure method and port range</td>
              <td className="px-4 py-3">Platform constant; endpoint is discovered into status</td>
            </tr>
            <tr className="border-t border-white/5">
              <td className="px-4 py-3">Ingress apps suffix</td>
              <td className="px-4 py-3">Platform-global fixed suffix</td>
            </tr>
            <tr className="border-t border-white/5">
              <td className="px-4 py-3">Default container requests</td>
              <td className="px-4 py-3">LimitRange injected by the Foundation Work</td>
            </tr>
            <tr className="border-t border-white/5">
              <td className="px-4 py-3">Quota tier</td>
              <td className="px-4 py-3">VirtualNodeClass (the one thing tenants choose)</td>
            </tr>
          </tbody>
        </table>
      </div>

      <h2 id="glossary">Glossary</h2>
      <div className="not-prose my-6 overflow-hidden rounded-xl border border-white/10">
        <table className="w-full border-collapse text-left text-sm">
          <tbody className="text-muted-foreground">
            {[
              ["Management cluster", "Single-node K3s carrying the OCM Hub and the KubeCell controller; runs no tenant workloads."],
              ["ManagedCluster", "OCM registration object representing one Cell host one-to-one."],
              ["Cell", "One ManagedCluster plus its host K3s cluster; the failure and isolation boundary."],
              ["VirtualNodeClass", "Cluster-scoped quota tier: hard-limit numbers plus storage class."],
              ["VirtualCluster", "The child cluster instance; key fields immutable, rich status."],
              ["VirtualClusterPlan", "Side-effect-free preflight; one-shot evaluation, acceptance is not reservation."],
              ["Logical child node", "The single node in the child API, always named kubelet; a view, not a machine."],
              ["Reflected Pod", "Host Pod created by K3k mirroring a child Pod, with identical requests."],
              ["Cluster proxy / MSA", "OCM mechanisms; the only path by which the controller reads host APIs."],
              ["appsSuffix", "Platform-global external domain suffix; hostnames derive as <ingress>.<vc>.<suffix>."],
            ].map(([term, meaning]) => (
              <tr key={term} className="border-t border-white/5">
                <td className="w-52 px-4 py-3 font-medium text-foreground">{term}</td>
                <td className="px-4 py-3">{meaning}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <p>
        Next: <Link href="/docs/installation/">install the management plane and onboard a host</Link>
        , or jump straight to the{" "}
        <Link href="/docs/api-reference/">API reference</Link> for every field.
      </p>
    </Prose>
  );
}
