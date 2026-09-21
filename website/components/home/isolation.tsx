import { Lock, Network, ScanSearch, ShieldCheck, UserCheck, Waypoints } from "lucide-react";

import { MagicCard } from "@/components/magicui/magic-card";
import { SectionHeading } from "@/components/section";
import { isolationControls } from "@/lib/content";

const icons = [ShieldCheck, Lock, Network, ScanSearch, Waypoints, UserCheck];

export function IsolationGrid() {
  return (
    <section className="relative py-20 sm:py-24">
      <div className="container-page">
        <SectionHeading
          eyebrow="Isolation model"
          title="Isolation is enforced on the host, not promised in a README"
          description="Every cluster runs in its own space on the host with real Kubernetes guardrails. Nothing depends on developers behaving well."
        />

        <div className="mt-12 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {isolationControls.map((control, index) => {
            const Icon = icons[index % icons.length];
            return (
              <MagicCard
                key={control.title}
                className="h-full rounded-xl border border-white/10 bg-white/[0.02]"
                gradientColor="#1d3f73"
                gradientOpacity={0.5}
              >
                <div className="flex h-full flex-col gap-3 p-6">
                  <span className="inline-flex h-10 w-10 items-center justify-center rounded-xl border border-sky-400/20 bg-sky-500/10 text-sky-300">
                    <Icon className="h-5 w-5" />
                  </span>
                  <h3 className="text-base font-semibold text-foreground">{control.title}</h3>
                  <p className="text-sm leading-6 text-muted-foreground">{control.description}</p>
                </div>
              </MagicCard>
            );
          })}
        </div>
      </div>
    </section>
  );
}
