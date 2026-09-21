import Link from "next/link";

import { Callout, Prose } from "@/components/docs/callout";
import { CodeBlock } from "@/components/code-block";
import { buildMetadata } from "@/lib/metadata";

export const metadata = buildMetadata({
  title: "Host onboarding",
  description:
    "Onboard physical hosts as KubeCell Cells with four commands: doctor, join, accept, chart. Includes Cell generation, class drafting, appending workers, and a troubleshooting table.",
  path: "/docs/host-onboarding/",
  keywords: [
    "KubeCell host onboarding",
    "clusteradm join",
    "ManagedCluster accept",
    "kubecell-host chart",
    "suggest-class",
  ],
});

const doctor = `go run ./cmd/kubecell-installer doctor management --bundle <release-dir>
go run ./cmd/kubecell-installer doctor host --bundle <release-dir> \\
  --device huawei.com/Ascend910`;

const four = `# 1. join — host context
KUBECONFIG=<host-kubeconfig> clusteradm join \\
  --hub-token "$(clusteradm get token)" \\
  --hub-apiserver https://hub.example.com:6443 \\
  --cluster-name <cell-name> --wait

# 2. accept — management context (mandatory manual gate)
KUBECONFIG=<hub-kubeconfig> clusteradm accept --clusters <cell-name> --wait

# 3. chart — install the host chart
kubectl create ns kubecell-system
helm install kubecell-host ./charts/kubecell-host \\
  --namespace kubecell-system \\
  --set image.repository=<controller-repo> \\
  --set image.tag=<tag> --set image.pullPolicy=IfNotPresent

# 4. cell — generate from live discovery, then apply
go run ./cmd/kubecell-installer host --step=cell --cell <cell-name> \\
  --kubeconfig <host-kubeconfig> | grep -v '^#' > cell.yaml
kubectl apply -n kubecell-system -f cell.yaml`;

const suggest = `go run ./cmd/kubecell-installer suggest-class \\
  --cell-file cell.yaml --name <class-name>

# The draft reflects observed allocatable capacity.
# Adjust the numbers down to the tenant envelope you are comfortable with,
# then apply the VirtualNodeClass.`;

const append = `# Case B: append same-profile workers to an existing Cell
# 1. Join the existing host cluster as a K3s agent with an independent data directory.
#    This deletes no data on that machine and runs no OCM onboarding.
# 2. Re-apply profile and topology labels, wait for node Ready and platform DaemonSets.
# 3. Confirm the new node appears in inventory:
kubectl -n kubecell-system get cell <cell-name> \\
  -o jsonpath='{range .status.nodes[*]}{.name}{"\\t"}{.ready}{"\\n"}{end}'`;

const symptoms = [
  [
    "clusteradm init reports success but installs nothing",
    "Wrong kubeconfig context",
    "Re-run with an explicit KUBECONFIG= prefix",
  ],
  [
    "OCM components stuck in ImagePullBackOff",
    "Image not available from the configured mirror",
    "Fix the component PullSpec to a reachable registry",
  ],
  [
    "Preloaded image not recognized by kubelet",
    "Short tag or incomplete layers",
    "Re-tag with the fully qualified name and verify with crictl inspecti",
  ],
  [
    "Child CoreDNS Pending",
    "Third-party Pod without resources blocked by LimitRange",
    "Covered by the kubecell-defaults baseline in the Foundation Work",
  ],
  [
    "Child PVC reports no free storage",
    "Missing TopoLVM topology labels",
    "Re-apply topology.topolvm.io/node and topology.kubernetes.io/zone labels",
  ],
  [
    "TopoLVM CSINode has no driver",
    "Custom kubelet root directory not passed to the chart",
    "Pass it with --set on the host chart",
  ],
  [
    "Traefik :80 unreachable",
    "Conflicting Service or stale host-port binding",
    "Use hostNetwork via values, remove stale load-balancer helpers, clear stale NAT chains",
  ],
  [
    "Child Ingress returns 503",
    "Mirrored Endpoints carry a mismatched port name",
    "The controller strips port names; for older clusters clear the port name to \"\"",
  ],
  [
    "Volumes remain after VirtualCluster deletion",
    "Orphaned logical volumes after the child API is gone",
    "Remove only cluster-created volumes with lvremove; leave the volume group intact",
  ],
  [
    "Zero accelerators reported",
    "Device Plugin registration mismatch",
    "Check plugin Pod logs for registration failures before changing anything",
  ],
];

export default function HostOnboardingPage() {
  return (
    <Prose>
      <p className="font-mono text-xs uppercase tracking-[0.2em] text-sky-400">Getting started</p>
      <h1 className="mt-3 text-3xl font-semibold tracking-tight text-foreground sm:text-4xl">
        Host onboarding
      </h1>
      <p className="mt-5 text-base leading-7 text-muted-foreground">
        Humans run four commands. The installer owns the release bundle, the pinned versions, and the
        baseline. This page is the operational companion to the{" "}
        <a href="https://github.com/taosher/KubeCell/blob/main/docs/operations/host-onboarding.md">
          repository guide
        </a>
        .
      </p>

      <Callout variant="info" title="Conventions">
        <code>KUBECONFIG_HUB</code> points at the management cluster and{" "}
        <code>KUBECONFIG_HOST</code> at the host cluster. Always pass an explicit{" "}
        <code>KUBECONFIG=</code> so a command cannot target the wrong cluster.
      </Callout>

      <h2 id="prerequisites">0. Prerequisites (platform team, one time)</h2>
      <ul>
        <li>
          Pinned versions from the release: host K3s <code>v1.36.3+k3s1</code>, K3k{" "}
          <code>v1.2.0</code>, child K3s <code>v1.34.2+k3s1</code>, OCM Hub <code>v1.3.1</code>.
        </li>
        <li>Registry mirrors configured and verified with a small image pull.</li>
        <li>
          Profile and topology labels on every host node:{" "}
          <code>hardware.kubecell.io/profile</code>, <code>topology.topolvm.io/node</code>,{" "}
          <code>topology.kubernetes.io/zone</code>.
        </li>
        <li>Controller image available to both clusters, matching architecture.</li>
      </ul>

      <h2 id="doctor">1. doctor — read-only preflight</h2>
      <CodeBlock className="not-prose my-6" code={doctor} language="bash" filename="doctor.sh" />
      <p>
        Stop at the first failure. On the host side, confirm: nodes Ready, profile labels present,
        TopoLVM capacity available, and accelerator resources reported by the Device Plugin.
      </p>

      <h2 id="four-commands">2. The four commands</h2>
      <CodeBlock className="not-prose my-6" code={four} language="bash" filename="four-commands.sh" />
      <Callout variant="warning" title="Do not reuse a stale token">
        Between fetching the hub token and running <code>clusteradm join</code>, do not call{" "}
        <code>get token</code> twice — the older token may expire. The ManagedCluster name must equal
        the future Cell name; the generator enforces this.
      </Callout>

      <h2 id="class">3. Cell and class</h2>
      <p>
        The Cell comes from live discovery, so you only apply it. The class is drafted from observed
        allocatable capacity and then tuned down to the tenant envelope.
      </p>
      <CodeBlock className="not-prose my-6" code={suggest} language="bash" filename="suggest-class.sh" />

      <h2 id="append-workers">4. Appending same-profile workers (Case B)</h2>
      <p>
        Growing an existing Cell does not create a new Cell and does not run OCM onboarding. Join the
        new machine as a K3s agent of the same host cluster, label it, and confirm it shows up in
        inventory. Existing workloads are neither migrated nor interrupted.
      </p>
      <CodeBlock className="not-prose my-6" code={append} language="bash" filename="append-workers.sh" />

      <h2 id="troubleshooting">5. Troubleshooting quick reference</h2>
      <div className="not-prose my-6 overflow-hidden rounded-xl border border-white/10">
        <div className="overflow-x-auto">
          <table className="w-full min-w-[720px] border-collapse text-left text-sm">
            <thead>
              <tr className="bg-white/[0.03]">
                <th className="px-4 py-3 font-medium text-muted-foreground">Symptom</th>
                <th className="px-4 py-3 font-medium text-muted-foreground">Likely cause</th>
                <th className="px-4 py-3 font-medium text-muted-foreground">Action</th>
              </tr>
            </thead>
            <tbody className="text-muted-foreground">
              {symptoms.map(([symptom, cause, action]) => (
                <tr key={symptom} className="border-t border-white/5">
                  <td className="px-4 py-3 text-foreground/85">{symptom}</td>
                  <td className="px-4 py-3">{cause}</td>
                  <td className="px-4 py-3">{action}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      <p>
        Next: <Link href="/docs/first-virtualcluster/">create your first VirtualCluster</Link>.
      </p>
    </Prose>
  );
}
