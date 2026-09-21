import Link from "next/link";

import { Callout, Prose } from "@/components/docs/callout";
import { CodeBlock } from "@/components/code-block";
import { buildMetadata } from "@/lib/metadata";

export const metadata = buildMetadata({
  title: "Installation",
  description:
    "Install KubeCell: pinned versions, prerequisites, the management plane, OCM Hub, host onboarding, and baseline verification in the documented order.",
  path: "/docs/installation/",
  keywords: [
    "install KubeCell",
    "KubeCell installer",
    "OCM Hub installation",
    "K3s host onboarding",
    "kubecell-management chart",
  ],
});

const prereq = `# Pinned combination (do not replace silently)
#   host K3s      v1.36.3+k3s1
#   K3k           v1.2.0
#   child K3s     v1.34.2+k3s1
#   OCM Hub       v1.3.1

# Host nodes carry the profile label and TopoLVM topology labels
kubectl label node <node> hardware.kubecell.io/profile=<profile>
kubectl label node <node> topology.topolvm.io/node=<node>
kubectl label node <node> topology.kubernetes.io/zone=<node>

# Both clusters must be able to pull the controller image.
# Configure K3s registries.yaml mirrors first and verify with a small image.`;

const management = `# 1. CRDs
kubectl apply -f kubecell-release/manifests/crds

# 2. OCM Hub (pinned version from the bundle)
go run ./cmd/kubecell-installer management \\
  --bundle ./kubecell-release --apply --confirm

# 3. Verify the management plane
kubectl -n open-cluster-management get pods
kubectl -n kubecell-system get deploy`;

const hostJoin = `# Run against the host cluster
export KUBECONFIG=<host-kubeconfig>

KUBECONFIG=$KUBECONFIG clusteradm join \\
  --hub-token "$(clusteradm get token)" \\
  --hub-apiserver https://hub.example.com:6443 \\
  --cluster-name <cell-name> --wait`;

const hostAccept = `# Run against the management cluster
export KUBECONFIG=<hub-kubeconfig>

KUBECONFIG=$KUBECONFIG clusteradm accept --clusters <cell-name> --wait`;

const hostChart = `kubectl create ns kubecell-system   # namespaceCreate is false, create it first

helm install kubecell-host ./charts/kubecell-host \\
  --namespace kubecell-system \\
  --set image.repository=<controller-repo> \\
  --set image.tag=<tag> --set image.pullPolicy=IfNotPresent`;

const verify = `kubectl -n kubecell-system get cell,vnclass,vcp,vc

# Cell readiness: joined, fresh lease, fresh inventory
kubectl -n kubecell-system get cell cell1 \\
  -o jsonpath='{.status.phase}{"\\t"}{.status.managedCluster.leaseFresh}{"\\n"}'`;

const order = [
  {
    title: "Apply CRDs",
    body: "Install the four KubeCell CRDs from the release bundle. This is the only step that must happen before anything else.",
  },
  {
    title: "Install the OCM Hub",
    body: "The management controller writes ManifestWork objects into the hub, so the hub must exist and be pinned to the bundle version.",
  },
  {
    title: "Install the management chart",
    body: "Deploys the controller, webhook, RBAC, and the optional Kite dashboard. Requires both --apply and --confirm.",
  },
  {
    title: "Join the host",
    body: "Run the locked clusteradm join command on the host. The ManagedCluster name must equal the future Cell name.",
  },
  {
    title: "Approve the ManagedCluster",
    body: "clusteradm accept is a mandatory manual safety gate. KubeCell never auto-approves a host.",
  },
  {
    title: "Install the host chart and baseline",
    body: "The host chart installs the webhook; the installer installs the pinned baseline: K3k, TopoLVM, Traefik, Device Plugin.",
  },
  {
    title: "Generate the Cell and verify",
    body: "host --step=cell produces the Cell from live discovery. Apply it, then confirm joined, fresh lease, and fresh inventory.",
  },
  {
    title: "Create class, plan, and child",
    body: "Draft a quota tier with suggest-class, then follow the first VirtualCluster guide.",
  },
];

export default function InstallationPage() {
  return (
    <Prose>
      <p className="font-mono text-xs uppercase tracking-[0.2em] text-sky-400">Getting started</p>
      <h1 className="mt-3 text-3xl font-semibold tracking-tight text-foreground sm:text-4xl">
        Installation
      </h1>
      <p className="mt-5 text-base leading-7 text-muted-foreground">
        KubeCell installs in a fixed order. Deviating from it is the most common cause of confusing
        failures, so treat this page as a checklist rather than a menu.
      </p>

      <Callout variant="warning" title="Pre-release">
        KubeCell is not production ready. Install it in a lab with disposable hosts until the
        hardening milestones complete.
      </Callout>

      <h2 id="prerequisites">Prerequisites</h2>
      <p>
        Both clusters must be reachable from your workstation, able to pull the controller image, and
        running the pinned versions. Host nodes need profile and TopoLVM topology labels — labels do
        not survive a node rebuild, so re-apply them after any reinstall.
      </p>
      <CodeBlock className="not-prose my-6" code={prereq} language="bash" filename="prerequisites.sh" />

      <h2 id="order">Installation order</h2>
      <ol>
        {order.map((step) => (
          <li key={step.title}>
            <strong>{step.title}.</strong> {step.body}
          </li>
        ))}
      </ol>

      <h2 id="management">Management plane</h2>
      <p>
        The management cluster is a single-node K3s that holds desired state and observations only.
        It runs no tenant workloads. Its K3s version may differ from the hosts.
      </p>
      <CodeBlock className="not-prose my-6" code={management} language="bash" filename="management.sh" />
      <Callout variant="info" title="Dry-run by default">
        The installer is read-only unless you pass both <code>--apply</code> and{" "}
        <code>--confirm</code>. <code>doctor</code> never mutates anything.
      </Callout>

      <h2 id="host">Host onboarding</h2>
      <p>
        Joining is a two-step operation with a manual approval in between. The approval exists so a
        compromised or misconfigured host cannot register itself into your fleet unnoticed.
      </p>
      <CodeBlock className="not-prose my-6" code={hostJoin} language="bash" filename="join.sh" />
      <CodeBlock className="not-prose my-6" code={hostAccept} language="bash" filename="accept.sh" />
      <CodeBlock className="not-prose my-6" code={hostChart} language="bash" filename="host-chart.sh" />
      <p>
        The installer then installs the host baseline at pinned versions, idempotently: K3k,
        TopoLVM, Traefik, the vendor Device Plugin, and profile labels. The controller never installs
        these itself.
      </p>

      <h2 id="cell">Generate the Cell</h2>
      <p>
        Do not hand-write a Cell from memory. Generate it from live host discovery so the machine
        profile matches reality, then apply it.
      </p>
      <CodeBlock
        className="not-prose my-6"
        language="bash"
        filename="cell.sh"
        code={`go run ./cmd/kubecell-installer host --step=cell --cell <cell-name> \\
  --kubeconfig <host-kubeconfig> | grep -v '^#' > cell.yaml

kubectl apply -n kubecell-system -f cell.yaml`}
      />
      <CodeBlock className="not-prose my-6" code={verify} language="bash" filename="verify.sh" />

      <h2 id="dashboard">Optional: Kite dashboard</h2>
      <p>
        The <code>kubecell-management</code> chart bundles the kite-org dashboard as a conditional
        dependency (<code>kite.enabled</code>, on by default). It runs single-node with sqlite,
        exposes a NodePort Service, and uses a dedicated ServiceAccount. Pass the initial superuser
        password at install time; never commit it.
      </p>

      <h2 id="uninstall">Uninstall and boundaries</h2>
      <ul>
        <li>
          The installer only touches the selected KubeCell release. OCM Hub,{" "}
          <code>ManagedCluster</code>, and klusterlet follow the OCM lifecycle and are out of Helm
          uninstall scope.
        </li>
        <li>
          Uninstalling a Helm release is not the same as uninstalling OCM. Remove registrations
          deliberately.
        </li>
        <li>
          Deleting a VirtualCluster is safe and finalizer-scoped; deleting the management plane while
          child clusters exist is not a supported cleanup path.
        </li>
      </ul>

      <p>
        Next: <Link href="/docs/host-onboarding/">host onboarding in four commands</Link> or{" "}
        <Link href="/docs/first-virtualcluster/">create your first VirtualCluster</Link>.
      </p>
    </Prose>
  );
}
