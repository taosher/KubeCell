import Link from "next/link";

import { Callout, Prose } from "@/components/docs/callout";
import { CodeBlock } from "@/components/code-block";
import { buildMetadata } from "@/lib/metadata";

export const metadata = buildMetadata({
  title: "Security and isolation",
  description:
    "The KubeCell isolation model: per-child host namespaces, ResourceQuota and LimitRange enforcement, default-deny NetworkPolicy, admission rejection of privileged workloads and hostPath volumes, and the credential model.",
  path: "/docs/security/",
  keywords: [
    "KubeCell security",
    "Kubernetes multi-tenancy",
    "ResourceQuota isolation",
    "NetworkPolicy child cluster",
    "hostPath rejection",
  ],
});

const rejection = `# What the host admission layer rejects in tenant workloads
#   - privileged containers
#   - host namespaces (hostNetwork, hostPID, hostIPC)
#   - hostPath volumes (with a replacement hint pointing at the storage class)
#   - host device access outside the declared extended resources
#
# What it does not do: inspect container images, enforce vendor runtimes,
# or act as an adversarial sandbox. See "Trust model" below.`;

const quotaExample = `apiVersion: v1
kind: ResourceQuota
metadata:
  name: kubecell-<vc>-quota
  namespace: <vc>-ns
spec:
  hard:
    requests.cpu: "4"
    limits.cpu: "4"
    requests.memory: 8Gi
    limits.memory: 8Gi
    requests.huawei.com/Ascend910: "2"
    limits.huawei.com/Ascend910: "2"`;

export default function SecurityPage() {
  return (
    <Prose>
      <p className="font-mono text-xs uppercase tracking-[0.2em] text-sky-400">Guides</p>
      <h1 className="mt-3 text-3xl font-semibold tracking-tight text-foreground sm:text-4xl">
        Security and isolation
      </h1>
      <p className="mt-5 text-base leading-7 text-muted-foreground">
        KubeCell gives every child cluster a real API server without giving tenants control of the
        host. Isolation is enforced by Kubernetes primitives on the host, plus a small admission
        layer that closes the obvious escalation paths.
      </p>

      <Callout variant="warning" title="Trust model">
        This is not strong-adversary multi-tenancy. Developer credentials are child-cluster admin
        kubeconfigs, and the network boundary assumes trusted or at least accountable tenants. Do not
        use KubeCell to host untrusted third-party code.
      </Callout>

      <h2 id="layers">The five isolation layers</h2>
      <ol>
        <li>
          <strong>Dedicated host namespace.</strong> Each child cluster maps to one host namespace
          with policy labels and annotations. Reflected Pods and PVCs land there, and nothing else
          does.
        </li>
        <li>
          <strong>ResourceQuota.</strong> The VirtualNodeClass entitlement becomes a hard quota on
          the host namespace. Requests and limits are both enforced, so a child cluster cannot
          overcommit the host.
        </li>
        <li>
          <strong>LimitRange.</strong> A default of 100m CPU and 256Mi memory applies to containers
          that declare nothing, which prevents platform sidecars in the child cluster from landing
          unschedulable.
        </li>
        <li>
          <strong>NetworkPolicy.</strong> Cross-namespace traffic is denied by default. Only the child
          API server path and required platform flows are allowed.
        </li>
        <li>
          <strong>Admission rejection.</strong> Privileged containers, host namespaces, and hostPath
          volumes are rejected in tenant workloads before the host ever schedules them.
        </li>
      </ol>
      <CodeBlock className="not-prose my-6" code={quotaExample} language="yaml" filename="quota.yaml" />
      <CodeBlock className="not-prose my-6" code={rejection} language="text" filename="admission.txt" />

      <h2 id="management">Management-plane boundary</h2>
      <p>
        The controller is intentionally weak by design. It can write OCM <code>ManifestWork</code>{" "}
        objects and read hosts through the OCM cluster proxy. It does not hold host kubeconfigs and
        does not open connections to host APIs.
      </p>
      <ul>
        <li>
          Every host-side change is a declarative work object, which makes changes reviewable and
          retries idempotent.
        </li>
        <li>
          A compromised controller can disrupt the fleet, but it cannot silently edit host state
          without leaving ManifestWork artifacts.
        </li>
        <li>
          Host onboarding requires a manual <code>clusteradm accept</code>. KubeCell never
          auto-approves a ManagedCluster.
        </li>
      </ul>

      <h2 id="accelerators">Accelerator isolation</h2>
      <ul>
        <li>
          Extended resources are allocated as whole cards: request equals limit, integer values.
          Sharing one accelerator between child clusters is not supported.
        </li>
        <li>
          Observed allocatable is the source of truth. A Cell declaration that exceeds observed
          capacity produces a <code>CapabilityMismatch</code> condition instead of oversubscription.
        </li>
        <li>
          KubeCell does not manage vendor runtimes, CDI, or Device Plugin health. Those remain host
          platform responsibilities, and diagnostics go through host Pod status and logs.
        </li>
      </ul>

      <h2 id="credentials">Credentials</h2>
      <ul>
        <li>
          Each VirtualCluster always publishes an admin kubeconfig to a management-plane Secret. There
          is no credential-less mode.
        </li>
        <li>
          Treat the Secret as cluster-admin material for the child cluster. Store it in your secret
          manager and use mode 600 for files.
        </li>
        <li>
          Revocation is deletion: removing the VirtualCluster removes the child API and its
          credentials. Rotate by recreating the child cluster.
        </li>
      </ul>

      <h2 id="storage">Storage and data</h2>
      <ul>
        <li>
          Child volumes live on host TopoLVM logical volumes. Deleting a child cluster does not
          automatically destroy data under the <code>Retain</code> policy.
        </li>
        <li>
          Reclaim is a deliberate operator action (<code>lvremove</code> for cluster-created volumes
          only). Never remove the volume group.
        </li>
        <li>
          <code>hostPath</code> is rejected because it would let a child workload read host data
          outside its namespace boundary.
        </li>
      </ul>

      <h2 id="reporting">Reporting a security issue</h2>
      <p>
        Do not open a public issue for a suspected vulnerability. Contact the maintainers through the
        repository&apos;s security reporting channel and include reproduction steps, the affected
        version, and the observed versus expected behavior.
      </p>

      <p>
        See also: <Link href="/architecture/">technical principles</Link> for the control flow and{" "}
        <Link href="/docs/operations/">day-2 operations</Link> for credential handling routines.
      </p>
    </Prose>
  );
}
