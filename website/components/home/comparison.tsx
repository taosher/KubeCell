import { Check, X } from "lucide-react";

import { SectionHeading } from "@/components/section";
import { cn } from "@/lib/utils";

type Row = {
  aspect: string;
  namespaces: string;
  namespacesGood: boolean;
  vmPerDev: string;
  vmGood: boolean;
  kubecell: string;
  kubecellGood: boolean;
};

const rows: Row[] = [
  {
    aspect: "API surface",
    namespaces: "Shared API server, shared cluster-wide resources",
    namespacesGood: false,
    vmPerDev: "Own API, heavy per-VM control plane",
    vmGood: true,
    kubecell: "Own K3s API server per developer cluster",
    kubecellGood: true,
  },
  {
    aspect: "Isolation",
    namespaces: "Namespaces and RBAC only; shared kernel",
    namespacesGood: false,
    vmPerDev: "Strong VM boundary, high fixed cost",
    vmGood: true,
    kubecell: "Dedicated namespace, hard quota, network policy",
    kubecellGood: true,
  },
  {
    aspect: "Accelerators",
    namespaces: "Shared allocation, hard to reason about",
    namespacesGood: false,
    vmPerDev: "GPU passthrough per VM, limited density",
    vmGood: false,
    kubecell: "Whole cards per cluster, never shared",
    kubecellGood: true,
  },
  {
    aspect: "Density and cost",
    namespaces: "High density, weak boundaries",
    namespacesGood: true,
    vmPerDev: "Low density, duplicated control planes",
    vmGood: false,
    kubecell: "Shared hosts, isolated control planes",
    kubecellGood: true,
  },
  {
    aspect: "Day-2 operations",
    namespaces: "One cluster to upgrade",
    namespacesGood: true,
    vmPerDev: "Thousands of VMs to patch",
    vmGood: false,
    kubecell: "One management plane to operate",
    kubecellGood: true,
  },
];

function Cell({ value, good }: { value: string; good: boolean }) {
  return (
    <div className="flex items-start gap-2">
      {good ? (
        <Check className="mt-0.5 h-4 w-4 shrink-0 text-emerald-400" />
      ) : (
        <X className="mt-0.5 h-4 w-4 shrink-0 text-rose-400/80" />
      )}
      <span className="text-sm leading-6 text-muted-foreground">{value}</span>
    </div>
  );
}

export function Comparison() {
  return (
    <section className="relative py-20 sm:py-24">
      <div className="container-page">
        <SectionHeading
          eyebrow="Trade-offs"
          title="Not namespaces. Not a VM per developer."
          description="KubeCell sits between the two familiar extremes: real cluster semantics per developer, sharing the hardware underneath."
        />

        <div className="mt-12 overflow-hidden rounded-2xl border border-white/10">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[720px] border-collapse text-left">
              <thead>
                <tr className="bg-white/[0.03] text-sm">
                  <th className="px-5 py-4 font-medium text-muted-foreground">Aspect</th>
                  <th className="px-5 py-4 font-medium text-muted-foreground">Shared namespaces</th>
                  <th className="px-5 py-4 font-medium text-muted-foreground">VM per developer</th>
                  <th
                    className={cn(
                      "px-5 py-4 font-semibold text-foreground",
                      "border-x border-sky-400/20 bg-sky-500/[0.07]"
                    )}
                  >
                    KubeCell
                  </th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <tr key={row.aspect} className="border-t border-white/5">
                    <td className="px-5 py-4 text-sm font-medium text-foreground">{row.aspect}</td>
                    <td className="px-5 py-4">
                      <Cell value={row.namespaces} good={row.namespacesGood} />
                    </td>
                    <td className="px-5 py-4">
                      <Cell value={row.vmPerDev} good={row.vmGood} />
                    </td>
                    <td className="border-x border-sky-400/20 bg-sky-500/[0.07] px-5 py-4">
                      <Cell value={row.kubecell} good={row.kubecellGood} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </section>
  );
}
