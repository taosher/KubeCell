import Link from "next/link";

import { Callout, Prose } from "@/components/docs/callout";
import { CodeBlock } from "@/components/code-block";
import { buildMetadata } from "@/lib/metadata";

export const metadata = buildMetadata({
  title: "Day-2 operations",
  description:
    "Operate KubeCell: health checks, credential handling, quota changes, ingress and DNS, storage lifecycle, upgrades, host maintenance, and cleanup.",
  path: "/docs/operations/",
  keywords: [
    "KubeCell operations",
    "VirtualCluster quota change",
    "KubeCell upgrade",
    "host maintenance Kubernetes",
    "kubeconfig rotation",
  ],
});

const health = `# Fleet overview: phase, OCM state, lease freshness
kubectl -n kubecell-system get cells \\
  -o custom-columns=NAME:.metadata.name,PHASE:.status.phase,\\
JOINED:.status.managedCluster.joined,AVAILABLE:.status.managedCluster.available,\\
LEASE:.status.managedCluster.leaseFresh,NODES:.status.nodes[*].name

# Child clusters and their endpoints
kubectl -n kubecell-system get vc \\
  -o custom-columns=NAME:.metadata.name,PHASE:.status.phase,\\
CELL:.spec.cellRef.name,CLASS:.spec.classRef.name,ENDPOINT:.status.endpoint.address

# Is inventory fresh enough to make promises?
kubectl -n kubecell-system get cell cell1 \\
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{"\\n"}{end}'`;

const quota = `# 1. Publish a new tier (platform action)
kubectl apply -f virtualnodeclass-large.yaml

# 2. Create a new child cluster on the larger tier
kubectl apply -f child-dev-large.yaml

# 3. Move workloads at your own pace, then delete the old cluster.
#    Existing clusters keep the resolution snapshot they were built from;
#    KubeCell never mutates a running child cluster in place.`;

const credentials = `# Find the published credential
kubectl -n kubecell-system get vc child-dev \\
  -o jsonpath='{.status.credential.secretName}{"\\n"}'

# Endpoint changed? The controller republishes the kubeconfig.
# Re-fetch the Secret rather than editing the file by hand.
kubectl -n kubecell-system get secret <secret-name> \\
  -o jsonpath='{.data.kubeconfig\\.yaml}' | base64 -d > child-dev.yaml
chmod 600 child-dev.yaml`;

const ingress = `# In the child cluster: standard Ingress, class kubecell
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web
  namespace: default
spec:
  ingressClassName: kubecell
  rules:
  - host: web.child-dev.apps.example.com   # <ingress>.<vc>.<apps-suffix>
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: web
            port:
              number: 80`;

const upgrade = `# Version drift is reported, not silently fixed
kubectl -n kubecell-system get cell cell1 \\
  -o jsonpath='{.status.provider.k3kVersion}{"\\n"}'

# Upgrades move through releases:
#   1. Build/verify a new release bundle (digests + pinned versions).
#   2. doctor against the current fleet.
#   3. Apply the new management chart, then the host baseline per Cell.
#   4. Watch Cell conditions and recreate child clusters only if the
#      release notes require it.`;

const maintenance = `# Planned host maintenance (v1 has no live migration)
# 1. Stop creating new child clusters on the Cell (tell tenants, or
#    temporarily remove the class from your self-service flow).
# 2. Delete or migrate child clusters deliberately; do not drain the host
#    node under running reflected Pods and expect a graceful move.
# 3. Perform maintenance, re-apply labels, verify the lease renews.
# 4. Confirm inventory freshness before reopening the Cell.`;

export default function OperationsPage() {
  return (
    <Prose>
      <p className="font-mono text-xs uppercase tracking-[0.2em] text-sky-400">Guides</p>
      <h1 className="mt-3 text-3xl font-semibold tracking-tight text-foreground sm:text-4xl">
        Day-2 operations
      </h1>
      <p className="mt-5 text-base leading-7 text-muted-foreground">
        KubeCell is designed to be operated from status, not from tribal knowledge. This page covers
        the routines that keep a fleet healthy and the actions that are deliberately manual.
      </p>

      <h2 id="health">Routine health checks</h2>
      <p>
        Two questions matter every day: are the Cells alive, and is the inventory fresh enough to
        make promises? The second is easy to forget and the cause of most &quot;the Plan lied&quot;
        reports.
      </p>
      <CodeBlock className="not-prose my-6" code={health} language="bash" filename="health.sh" />
      <Callout variant="info" title="Lease freshness is the liveness signal">
        OCM <code>Available=True</code> has been observed to go stale without updates. Treat a fresh{" "}
        <code>managed-cluster-lease</code> as the decisive signal, and <code>InventoryFresh</code> as
        the precondition for any capacity decision.
      </Callout>

      <h2 id="quota">Changing a child cluster&apos;s size</h2>
      <p>
        There is no in-place resize. Vertical scaling is a deliberate move to a different quota tier:
        publish the new class, create the new child cluster, move workloads, delete the old one.
      </p>
      <CodeBlock className="not-prose my-6" code={quota} language="bash" filename="quota.sh" />
      <Callout variant="tip" title="Why no in-place resize">
        Quota changes ripple into the Foundation Work, the child control-plane size, and the
        resolution snapshot. Recreating keeps every step observable and reversible; patching would
        make partial failure states hard to reason about.
      </Callout>

      <h2 id="credentials">Credentials and endpoint changes</h2>
      <p>
        The child admin kubeconfig is always published. When the discovered host address changes, the
        controller rewrites and republishes the Secret — clients should re-fetch rather than pin an
        address.
      </p>
      <CodeBlock
        className="not-prose my-6"
        code={credentials}
        language="bash"
        filename="credentials.sh"
      />
      <Callout variant="warning" title="Credentials are cluster-admin">
        Child kubeconfigs grant admin on the child cluster. Distribute them through your secret
        manager, keep file modes at 600, and revoke by deleting the VirtualCluster.
      </Callout>

      <h2 id="ingress">Ingress and DNS</h2>
      <p>
        Host Traefik owns ports 80 and 443 for the entire host. It watches only VirtualCluster
        namespaces carrying the KubeCell label. Developers choose the Ingress name; the rest of the
        hostname is derived.
      </p>
      <CodeBlock className="not-prose my-6" code={ingress} language="yaml" filename="ingress.yaml" />
      <p>
        DNS automation is enabled when a second host arrives: point a wildcard record for{" "}
        <code>*.&lt;vc&gt;.&lt;apps-suffix&gt;</code> at the Cell&apos;s ingress address. Ingress
        routing is outside the locked-combination commitments, so verify it in your environment
        before promising it to tenants.
      </p>

      <h2 id="storage">Storage lifecycle</h2>
      <ul>
        <li>
          Child PVCs are reflected to host PVCs on TopoLVM. If a child PVC does not bind, check
          topology labels first, then TopoLVM health.
        </li>
        <li>
          Under <code>Retain</code>, deleting a VirtualCluster leaves PVs <code>Released</code> and
          data intact. Reclaim deliberately with <code>lvremove</code> for cluster-created volumes
          only; never remove the volume group.
        </li>
        <li>
          Storage free space is read from TopoLVM/lvmd. Inferring it from PV capacity is explicitly
          forbidden, so treat <code>freeCapacityKnown=false</code> as &quot;unknown&quot;, not
          &quot;zero&quot;.
        </li>
      </ul>

      <h2 id="upgrades">Upgrades and version pinning</h2>
      <p>
        Versions move through releases. The controller reports observed versions; it never upgrades
        anything on its own.
      </p>
      <CodeBlock className="not-prose my-6" code={upgrade} language="bash" filename="upgrade.sh" />

      <h2 id="maintenance">Planned host maintenance</h2>
      <p>
        The first version does not migrate running child clusters between hosts. Plan maintenance
        windows accordingly and communicate them to tenants.
      </p>
      <CodeBlock
        className="not-prose my-6"
        code={maintenance}
        language="bash"
        filename="maintenance.sh"
      />

      <h2 id="cleanup">Cleanup checklist</h2>
      <ul>
        <li>Delete child clusters you no longer need; the finalizer cleans the host side.</li>
        <li>Check for <code>Released</code> PVs and decide reclaim vs. retain per volume.</li>
        <li>Remove unused VirtualNodeClasses so tenants cannot request dead tiers.</li>
        <li>
          Remove the OCM <code>ManagedCluster</code> registration explicitly if a host leaves the
          fleet; Helm uninstall does not do it for you.
        </li>
      </ul>

      <p>
        Next: <Link href="/docs/api-reference/">API reference</Link> or{" "}
        <Link href="/docs/troubleshooting/">troubleshooting</Link>.
      </p>
    </Prose>
  );
}
