import Link from "next/link";

import { Callout, Prose } from "@/components/docs/callout";
import { CodeBlock } from "@/components/code-block";
import { buildMetadata } from "@/lib/metadata";

export const metadata = buildMetadata({
  title: "Troubleshooting",
  description:
    "Symptom-first KubeCell troubleshooting: Cell not Ready, ManifestWork not Applied, child API unreachable, PVCs not binding, accelerators missing, and ingress failures.",
  path: "/docs/troubleshooting/",
  keywords: [
    "KubeCell troubleshooting",
    "Cell not ready",
    "ManifestWork not applied",
    "child cluster API unreachable",
    "TopoLVM PVC pending",
  ],
});

const walkChain = `# 1. Management plane: what does the controller believe?
kubectl -n kubecell-system get vc <vc> -o yaml
kubectl -n kubecell-system describe vc <vc>

# 2. OCM: did the work get applied?
kubectl get manifestwork -n <managed-cluster-namespace>
kubectl get manifestwork -n <managed-cluster-namespace> <work> -o yaml

# 3. Host: is the work-agent healthy and did it land?
KUBECONFIG=<host-kubeconfig> kubectl -n open-cluster-management-agent \\
  get pods
KUBECONFIG=<host-kubeconfig> kubectl -n <vc>-ns get all,quota,limitrange,netpol

# 4. Child: is the control plane up and reachable?
KUBECONFIG=<host-kubeconfig> kubectl -n k3k-<vc> get pods,svc,pvc`;

const rows: Array<[string, string, string]> = [
  [
    "Cell not Ready",
    "Check in order: OCM Joined/Available, then managed-cluster-lease freshness, then Cell conditions. Available=True alone is not trusted.",
    "If the lease is stale, treat the host as offline and stop creating child clusters on it.",
  ],
  [
    "Cell Degraded but OCM looks fine",
    "Lease freshness or an inventory source failed.",
    "Inspect conditions: ProviderReady and InventoryFresh. Fix the reported subsystem before anything else.",
  ],
  [
    "Work not Applied",
    "ManifestWork status, then host work-agent, then the target namespace.",
    "Check namespace quota and admission rejections — a rejected manifest will retry forever.",
  ],
  [
    "VirtualCluster stuck Provisioning",
    "Foundation Work applied but Instance Work blocked, or K3k cannot schedule its server Pod.",
    "Check K3k controller health, server Pod events, and TopoLVM PVC binding on the host.",
  ],
  [
    "Child API unreachable",
    "status.endpoint versus the published Secret, then host NodePort Service.",
    "Re-fetch the Secret after endpoint changes; verify the NodePort is allocated and fresh.",
  ],
  [
    "Child PVC Pending",
    "Topology labels missing or TopoLVM has no free capacity.",
    "Re-apply topology.topolvm.io/node and topology.kubernetes.io/zone, then check lvmd reporting.",
  ],
  [
    "Zero accelerators visible",
    "Device Plugin registration mismatch.",
    "Read plugin Pod logs on the host before changing any declaration; observed allocatable wins.",
  ],
  [
    "CapabilityMismatch condition",
    "Cell declares more than the host allocates.",
    "Align the machine profile with the observed resource key and capacity, then re-observe.",
  ],
  [
    "Tenant Pod rejected",
    "Privileged access or hostPath in the manifest.",
    "This is intended. Use the rejection hint and switch to a PVC on the storage class.",
  ],
  [
    "Ingress returns 503",
    "Mirrored Endpoints carry a port name that does not match.",
    "The controller strips port names; for older clusters clear the endpoint port name to an empty string.",
  ],
  [
    "Image pull failures",
    "Registry reachability or OCI-archive transfer problems.",
    "Fix mirrors first; helpers live in hack/image/. Record digests and architectures.",
  ],
  [
    "Volumes remain after deletion",
    "Orphaned logical volumes once the child API is gone.",
    "Remove only cluster-created volumes with lvremove; leave the volume group intact.",
  ],
];

export default function TroubleshootingPage() {
  return (
    <Prose>
      <p className="font-mono text-xs uppercase tracking-[0.2em] text-sky-400">Guides</p>
      <h1 className="mt-3 text-3xl font-semibold tracking-tight text-foreground sm:text-4xl">
        Troubleshooting
      </h1>
      <p className="mt-5 text-base leading-7 text-muted-foreground">
        KubeCell spans four systems — the management plane, OCM, the host cluster, and the child K3s
        cluster. Almost every failure is diagnosed by walking that chain in order instead of guessing.
      </p>

      <h2 id="chain">Walk the chain</h2>
      <CodeBlock className="not-prose my-6" code={walkChain} language="bash" filename="diagnose.sh" />
      <Callout variant="tip" title="One rule that saves time">
        Never edit host state by hand to &quot;unstick&quot; a child cluster. Fix the input (the
        manifest, the class, the inventory source) and let reconciliation re-render from the snapshot.
        Manual edits are overwritten on the next reconcile and hide the real cause.
      </Callout>

      <h2 id="symptoms">Symptom reference</h2>
      <div className="not-prose my-6 overflow-hidden rounded-xl border border-white/10">
        <div className="overflow-x-auto">
          <table className="w-full min-w-[760px] border-collapse text-left text-sm">
            <thead>
              <tr className="bg-white/[0.03]">
                <th className="px-4 py-3 font-medium text-muted-foreground">Symptom</th>
                <th className="px-4 py-3 font-medium text-muted-foreground">Where to look</th>
                <th className="px-4 py-3 font-medium text-muted-foreground">Action</th>
              </tr>
            </thead>
            <tbody className="text-muted-foreground">
              {rows.map(([symptom, look, action]) => (
                <tr key={symptom} className="border-t border-white/5">
                  <td className="px-4 py-3 text-foreground/85">{symptom}</td>
                  <td className="px-4 py-3">{look}</td>
                  <td className="px-4 py-3">{action}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      <h2 id="plan-rejected">A Plan was rejected — now what?</h2>
      <ol>
        <li>
          Read <code>status.checks</code>. Each failed item names the resource that could not be
          satisfied.
        </li>
        <li>
          Check <code>expiresAt</code>. If the conclusion expired, re-apply the Plan for a fresh
          evaluation.
        </li>
        <li>
          If the missing resource is storage or accelerators, verify that the Cell inventory is fresh
          before concluding the host is full.
        </li>
        <li>
          Adjust the request or switch Cells. Do not create the VirtualCluster &quot;to see what
          happens&quot; — the controller rechecks and blocks anyway.
        </li>
      </ol>

      <h2 id="collect">Collecting evidence for an issue</h2>
      <ul>
        <li>Cell and VirtualCluster YAML (including status) with timestamps.</li>
        <li>ManifestWork status for the affected child cluster.</li>
        <li>Host events for the child namespace, plus K3k server Pod logs.</li>
        <li>Observed versions from Cell status versus the pinned release.</li>
        <li>Reproduction steps and expected versus actual behavior.</li>
      </ul>
      <p>
        File bugs at{" "}
        <a href="https://github.com/taosher/KubeCell/issues">github.com/taosher/KubeCell/issues</a>{" "}
        using the structure above. See also the{" "}
        <Link href="/docs/host-onboarding/">host onboarding troubleshooting table</Link> for
        host-side symptoms.
      </p>
    </Prose>
  );
}
