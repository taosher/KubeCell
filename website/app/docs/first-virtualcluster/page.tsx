import Link from "next/link";

import { Callout, Prose } from "@/components/docs/callout";
import { CodeBlock } from "@/components/code-block";
import { buildMetadata } from "@/lib/metadata";

export const metadata = buildMetadata({
  title: "Create your first VirtualCluster",
  description:
    "Create a Cell, draft a VirtualNodeClass, preflight with a VirtualClusterPlan, create a VirtualCluster, export the admin kubeconfig, and verify workloads, storage, and ingress.",
  path: "/docs/first-virtualcluster/",
  keywords: [
    "create VirtualCluster",
    "KubeCell kubeconfig",
    "KubeCell Plan Accepted",
    "child cluster verification",
  ],
});

const planYaml = `apiVersion: kubecell.io/v1alpha1
kind: VirtualClusterPlan
metadata:
  name: child-dev-plan
  namespace: kubecell-system
spec:
  cellRef: {name: cell1}
  classRef: {name: ascend-910b-small}`;

const vcYaml = `apiVersion: kubecell.io/v1alpha1
kind: VirtualCluster
metadata:
  name: child-dev
  namespace: kubecell-system
spec:
  cellRef: {name: cell1}
  classRef: {name: ascend-910b-small}`;

const kubeconfig = `VC=child-dev

# The Secret name embeds the VirtualCluster UID
SECRET=$(kubectl -n kubecell-system get vc $VC \\
  -o jsonpath='{.status.credential.secretName}')

kubectl -n kubecell-system get secret "$SECRET" \\
  -o jsonpath='{.data.kubeconfig\\.yaml}' | base64 -d > $VC.yaml
chmod 600 $VC.yaml

kubectl --kubeconfig $VC.yaml get nodes
kubectl --kubeconfig $VC.yaml get ns`;

const workload = `cat <<'EOF' | kubectl --kubeconfig child-dev.yaml apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: cpu-smoke
spec:
  containers:
  - name: app
    image: registry.k8s.io/pause:3.10
    resources:
      requests: {cpu: 100m, memory: 128Mi}
      limits: {cpu: 200m, memory: 256Mi}
EOF

# Confirm the reflected host Pod and quota accounting
kubectl -n kubecell-system get vc child-dev \\
  -o jsonpath='{.status.host.quotaUsed}{"\\n"}'`;

export default function FirstVirtualClusterPage() {
  return (
    <Prose>
      <p className="font-mono text-xs uppercase tracking-[0.2em] text-sky-400">Getting started</p>
      <h1 className="mt-3 text-3xl font-semibold tracking-tight text-foreground sm:text-4xl">
        Create your first VirtualCluster
      </h1>
      <p className="mt-5 text-base leading-7 text-muted-foreground">
        This guide assumes a Ready Cell and an existing quota tier. If you have not onboarded a host
        yet, start with <Link href="/docs/host-onboarding/">host onboarding</Link>.
      </p>

      <h2 id="prerequisites">Before you start</h2>
      <ul>
        <li>
          The target Cell is <strong>Ready</strong>: OCM <code>Joined=True</code>, a fresh{" "}
          <code>managed-cluster-lease</code>, and fresh inventory.
        </li>
        <li>
          The quota tier (<code>VirtualNodeClass</code>) you intend to use already exists.
        </li>
        <li>
          You are applying into <code>kubecell-system</code> on the management cluster, where the
          controller lives.
        </li>
      </ul>

      <h2 id="inspect">1. Inspect the Cell before requesting anything</h2>
      <p>
        Feasibility conclusions are only as good as the inventory they read. Check freshness first,
        then look at headroom.
      </p>
      <CodeBlock
        className="not-prose my-6"
        language="bash"
        filename="inspect.sh"
        code={`kubectl -n kubecell-system get cell cell1 -o yaml | less

# Things to look at in status:
#   conditions        -> InventoryFresh must be true
#   nodes[]           -> per-node capacity, allocatable, active/pending requests
#   inventory         -> aggregated available estimates per resource
#   storageInventory  -> freeCapacityKnown and freshness`}
      />
      <Callout variant="warning" title="Stop if inventory is stale">
        If <code>InventoryFresh</code> is false or key inventory is unknown, do not create anything.
        Contact operations; no feasibility conclusion is trustworthy at that point.
      </Callout>

      <h2 id="plan">2. Preflight with a Plan</h2>
      <p>
        Write a VirtualClusterPlan with exactly the content you intend to create. Wait for{" "}
        <code>status.decision</code> and read <code>checks</code> when it is rejected.
      </p>
      <CodeBlock className="not-prose my-6" code={planYaml} language="yaml" filename="plan.yaml" />
      <CodeBlock
        className="not-prose my-6"
        language="bash"
        filename="decision.sh"
        code={`kubectl apply -f plan.yaml
kubectl -n kubecell-system get vcp child-dev-plan \\
  -o jsonpath='{.status.decision}{"\\n"}'

# Rejected? Read the failed items and adjust the request or the Cell.
kubectl -n kubecell-system get vcp child-dev-plan \\
  -o jsonpath='{range .status.checks[*]}{.resource}{"\\t"}{.result}{"\\n"}{end}'`}
      />
      <p>
        <code>Accepted</code> means buildable under the current snapshot; <code>Rejected</code> means
        a specific resource is missing; <code>Unknown</code> means the inventory was insufficient to
        decide. Remember that acceptance is not reservation: the controller rechecks at creation
        time.
      </p>

      <h2 id="create">3. Create the VirtualCluster</h2>
      <p>
        The Plan and the VirtualCluster are isomorphic — the same file with a different{" "}
        <code>kind</code>. The webhook validates that references exist and quota keys are legal.
      </p>
      <CodeBlock className="not-prose my-6" code={vcYaml} language="yaml" filename="virtualcluster.yaml" />
      <CodeBlock
        className="not-prose my-6"
        language="bash"
        filename="wait.sh"
        code={`kubectl apply -f virtualcluster.yaml

# The first Ready transition typically takes 10-20 minutes (image pulls).
kubectl -n kubecell-system wait --for=condition=Ready vc/child-dev --timeout=25m
kubectl -n kubecell-system get vc child-dev -o wide`}
      />

      <h2 id="kubeconfig">4. Fetch the admin kubeconfig</h2>
      <p>
        The controller reads the K3k-generated kubeconfig through the OCM proxy, rewrites the server
        address to the discovered host address and NodePort, and publishes it as a Secret in the
        management plane. The Secret is always published; treat it as a credential.
      </p>
      <CodeBlock className="not-prose my-6" code={kubeconfig} language="bash" filename="kubeconfig.sh" />

      <h2 id="verify">5. Verify workloads, storage, and ingress</h2>
      <CodeBlock className="not-prose my-6" code={workload} language="bash" filename="workload.sh" />
      <ul>
        <li>
          The child cluster has exactly one node named <code>kubelet</code>.
        </li>
        <li>
          A child PVC becomes a host PVC backed by TopoLVM; check that it binds, not just that it is
          created.
        </li>
        <li>
          <code>hostPath</code> volumes are rejected at admission with a replacement hint — this is
          expected behavior, not a bug.
        </li>
        <li>
          Ingress: create a standard Ingress with class <code>kubecell</code> in the child cluster,
          then reach it at <code>http://&lt;ingress&gt;.&lt;vc&gt;.apps.example.com</code>.
        </li>
      </ul>

      <h2 id="delete">6. Delete it cleanly</h2>
      <CodeBlock
        className="not-prose my-6"
        language="bash"
        filename="delete.sh"
        code={`kubectl -n kubecell-system delete vc child-dev

# The finalizer removes both ManifestWorks by UID, which removes the host
# namespace, quota, policy, and the K3k cluster. Under Retain, PVs become
# Released and data is preserved.`}
      />

      <p>
        Next: <Link href="/docs/operations/">day-2 operations</Link>, or jump to the{" "}
        <Link href="/docs/troubleshooting/">troubleshooting guide</Link> if something is not Ready.
      </p>
    </Prose>
  );
}
