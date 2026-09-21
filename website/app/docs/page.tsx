import Link from "next/link";
import { ArrowRight } from "lucide-react";

import { Callout, Prose } from "@/components/docs/callout";
import { ArchitectureBeam } from "@/components/home/architecture-beam";
import { Badge } from "@/components/ui/badge";
import { docsFlat } from "@/lib/docs-nav";
import { buildMetadata } from "@/lib/metadata";

export const metadata = buildMetadata({
  title: "Documentation overview",
  description:
    "KubeCell documentation overview: what the operator is, the four CRDs, the three-layer architecture, design principles, and where to go next.",
  path: "/docs/",
  keywords: ["KubeCell documentation", "VirtualCluster", "KubeCell concepts"],
});

export default function DocsOverviewPage() {
  return (
    <Prose>
      <p className="font-mono text-xs uppercase tracking-[0.2em] text-sky-400">Documentation</p>
      <h1 className="mt-3 text-3xl font-semibold tracking-tight text-foreground sm:text-4xl">
        KubeCell overview
      </h1>
      <p className="mt-5 text-base leading-7 text-muted-foreground">
        KubeCell is a Kubernetes Operator that provisions isolated Kubernetes child clusters —{" "}
        <strong>VirtualClusters</strong> — on shared physical hosts. A single management cluster holds
        the desired state. Each Cell host cluster runs the workloads. Developers get a standard,
        kubectl-compatible child K3s API with hard quota isolation, reflected TopoLVM storage, and
        unified accelerator semantics.
      </p>

      <Callout variant="warning" title="Pre-release software">
        KubeCell is under active development and the APIs are <code>v1alpha1</code>. Do not use it
        for production workloads yet.
      </Callout>

      <h2 id="what-it-is">What KubeCell is</h2>
      <ul>
        <li>
          A small operator with exactly <strong>four CRDs</strong>: <code>Cell</code>,{" "}
          <code>VirtualNodeClass</code>, <code>VirtualClusterPlan</code>, and{" "}
          <code>VirtualCluster</code>.
        </li>
        <li>
          A composition layer over upstream projects — K3s, K3k, TopoLVM, Traefik, and Open Cluster
          Management (OCM) — integrated through public APIs only.
        </li>
        <li>
          A quota and isolation system: every child cluster maps to a host namespace with{" "}
          <code>ResourceQuota</code>, <code>LimitRange</code>, <code>NetworkPolicy</code>, and
          admission rules that reject privileged workloads and <code>hostPath</code> volumes.
        </li>
        <li>
          An inventory and feasibility system: Cells report live capacity, and Plans tell you whether
          a request can be satisfied <em>before</em> anything is created.
        </li>
      </ul>

      <h2 id="what-it-is-not">What KubeCell is not</h2>
      <ul>
        <li>
          Not a second scheduler, device allocator, kubelet, CNI, CSI, or Device Plugin. Those stay
          upstream.
        </li>
        <li>
          Not a namespace-with-labels product. A child cluster has its own API server, control plane,
          and data volume.
        </li>
        <li>
          Not a strong-adversary multi-tenancy boundary. Developer credentials are child-cluster admin
          kubeconfigs, which is a deliberate trusted-network trade-off.
        </li>
        <li>
          Not a horizontal autoscaler for child clusters: each child exposes exactly one logical node
          named <code>kubelet</code>. You scale by switching quota tiers.
        </li>
      </ul>

      <h2 id="mental-model">The mental model in four objects</h2>
      <div className="not-prose my-6 overflow-hidden rounded-xl border border-white/10">
        <table className="w-full border-collapse text-left text-sm">
          <thead>
            <tr className="bg-white/[0.03]">
              <th className="px-4 py-3 font-medium text-muted-foreground">Object</th>
              <th className="px-4 py-3 font-medium text-muted-foreground">Scope</th>
              <th className="px-4 py-3 font-medium text-muted-foreground">You use it to…</th>
            </tr>
          </thead>
          <tbody className="text-muted-foreground">
            <tr className="border-t border-white/5">
              <td className="px-4 py-3 font-mono text-xs text-sky-300">Cell</td>
              <td className="px-4 py-3">Namespaced</td>
              <td className="px-4 py-3">
                Bind one OCM <code>ManagedCluster</code> to one machine profile. Generated from live
                discovery.
              </td>
            </tr>
            <tr className="border-t border-white/5">
              <td className="px-4 py-3 font-mono text-xs text-sky-300">VirtualNodeClass</td>
              <td className="px-4 py-3">Cluster-scoped</td>
              <td className="px-4 py-3">
                Publish a quota tier: hard requests/limits per child cluster plus a storage class.
              </td>
            </tr>
            <tr className="border-t border-white/5">
              <td className="px-4 py-3 font-mono text-xs text-sky-300">VirtualClusterPlan</td>
              <td className="px-4 py-3">Namespaced</td>
              <td className="px-4 py-3">
                Check feasibility without side effects: <code>Accepted</code>, <code>Rejected</code>,
                or <code>Unknown</code>.
              </td>
            </tr>
            <tr className="border-t border-white/5">
              <td className="px-4 py-3 font-mono text-xs text-sky-300">VirtualCluster</td>
              <td className="px-4 py-3">Namespaced</td>
              <td className="px-4 py-3">
                Create the child cluster and receive a published admin kubeconfig in status.
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <h2 id="architecture">Architecture at a glance</h2>
      <p>
        Three layers, one direction of control. The management plane declares intent; the host
        executes it; the child cluster is the product.
      </p>
      <div className="not-prose my-8">
        <ArchitectureBeam />
      </div>
      <p>
        The controller never connects to a host directly. It applies two declarative work objects per
        cluster through the hub and reads host state back the same way. See{" "}
        <Link href="/architecture/">technical principles</Link> for the full control flow.
      </p>

      <h2 id="principles">Design principles worth remembering</h2>
      <ul>
        <li>
          <strong>Intent-only specs.</strong> Human-authored manifests declare which Cell and which
          quota tier. Versions, ports, reservations, and security baselines are platform constants and
          never enter the CRDs.
        </li>
        <li>
          <strong>One safe value means no field.</strong> If a setting has exactly one safe value,
          exposing it would only create drift, so it stays out of the API.
        </li>
        <li>
          <strong>Observed state beats declared state.</strong> Allocatable capacity reported by the
          host is the source of truth; declarations are upper bounds and never inflate inventory.
        </li>
        <li>
          <strong>Reconciliation is idempotent.</strong> Failed steps are retried by re-rendering from
          the resolution snapshot, never by patching host state in place.
        </li>
        <li>
          <strong>Built on upstream components.</strong> K3s, K3k, TopoLVM, and OCM are used
          unmodified through their public APIs, so upstream fixes and behavior apply directly.
        </li>
      </ul>

      <h2 id="who">Who should read what</h2>
      <div className="not-prose mt-6 grid gap-3 sm:grid-cols-2">
        {docsFlat.map((item) => (
          <Link
            key={item.href}
            href={item.href}
            className="group rounded-xl border border-white/10 bg-white/[0.02] p-4 transition-colors hover:border-sky-400/30"
          >
            <p className="text-sm font-medium text-foreground">{item.title}</p>
            <p className="mt-1 text-sm leading-6 text-muted-foreground">{item.description}</p>
            <span className="mt-2 inline-flex items-center gap-1 text-xs font-medium text-sky-400">
              Open
              <ArrowRight className="h-3 w-3 transition-transform group-hover:translate-x-0.5" />
            </span>
          </Link>
        ))}
      </div>

      <h2 id="status">Project status</h2>
      <p>
        The API shape, controllers, installer, and the first end-to-end loop are implemented. The
        feature matrix (PVC, networking, hostPath rejection, interaction, cascading deletion,
        accelerator allocation, Ingress) and release hardening are in progress. Track the work in{" "}
        <a href="https://github.com/taosher/KubeCell/blob/main/TODOS.md">TODOS.md</a> and report bugs
        through GitHub issues.
      </p>
      <div className="not-prose mt-4 flex flex-wrap gap-2">
        <Badge variant="outline" className="border-emerald-400/30 text-emerald-300">
          API shape frozen
        </Badge>
        <Badge variant="outline" className="border-sky-400/30 text-sky-300">
          Controllers implemented
        </Badge>
        <Badge variant="outline" className="border-amber-400/30 text-amber-300">
          Release hardening in progress
        </Badge>
      </div>
    </Prose>
  );
}
