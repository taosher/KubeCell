"use client";

import { CheckCircle2, ClipboardCheck, DownloadCloud, Rocket, Wrench } from "lucide-react";

import { AnimatedSpan, Terminal, TypingAnimation } from "@/components/magicui/terminal";
import { BlurFade } from "@/components/magicui/blur-fade";
import { SectionHeading } from "@/components/section";

const steps = [
  {
    icon: ClipboardCheck,
    title: "Check before you change anything",
    description:
      "A read-only preflight inspects both clusters and stops at the first problem. Nothing is modified until you pass --apply and --confirm.",
  },
  {
    icon: DownloadCloud,
    title: "Add your host",
    description:
      "Register the machine and approve it manually — a deliberate safety gate, so a new host can never join silently.",
  },
  {
    icon: Wrench,
    title: "Let KubeCell prepare the host",
    description:
      "Storage, ingress, and accelerator support are installed at tested versions. You do not assemble the stack yourself.",
  },
  {
    icon: Rocket,
    title: "Create the developer cluster",
    description:
      "Pick the host and a quota tier, apply an eight-line manifest, and collect the kubeconfig from the resource status.",
  },
];

export function Workflow() {
  return (
    <section className="relative py-20 sm:py-24">
      <div className="container-page">
        <SectionHeading
          eyebrow="How it works"
          title="Four commands to a running cluster"
          description="Humans run four commands. KubeCell handles the rest: what to install, which versions, and how to keep every host consistent."
        />

        <div className="mt-14 grid gap-12 lg:grid-cols-2 lg:items-center">
          <div className="space-y-6">
            {steps.map((step, index) => (
              <BlurFade key={step.title} delay={index * 0.08} inView>
                <div className="flex gap-4">
                  <div className="flex flex-col items-center">
                    <span className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border border-sky-400/20 bg-sky-500/10 text-sky-300">
                      <step.icon className="h-5 w-5" />
                    </span>
                    {index < steps.length - 1 ? (
                      <span className="mt-2 h-full w-px flex-1 bg-gradient-to-b from-sky-400/30 to-transparent" />
                    ) : null}
                  </div>
                  <div className="pb-2">
                    <h3 className="text-base font-semibold text-foreground">{step.title}</h3>
                    <p className="mt-1 text-sm leading-6 text-muted-foreground">
                      {step.description}
                    </p>
                  </div>
                </div>
              </BlurFade>
            ))}
            <BlurFade delay={0.4} inView>
              <div className="flex items-start gap-3 rounded-xl border border-emerald-400/15 bg-emerald-500/[0.06] p-4">
                <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-400" />
                <p className="text-sm leading-6 text-emerald-100/80">
                  The first cluster usually takes 10–20 minutes while container images are pulled onto
                  the host. Clusters created afterwards start much faster.
                </p>
              </div>
            </BlurFade>
          </div>

          <BlurFade delay={0.15} inView>
            <Terminal
              sequence
              className="max-w-none bg-[#070b14] shadow-[0_30px_80px_-40px_rgba(47,127,224,0.6)]"
            >
              <TypingAnimation delay={0} className="font-mono text-xs text-slate-300">
                $ go run ./cmd/kubecell-installer doctor host --bundle ./release
              </TypingAnimation>
              <AnimatedSpan delay={400} className="font-mono text-xs text-emerald-400">
                ✓ host reachable and healthy
              </AnimatedSpan>
              <AnimatedSpan delay={300} className="font-mono text-xs text-emerald-400">
                ✓ machine profile detected: ascend-910b
              </AnimatedSpan>
              <AnimatedSpan delay={300} className="font-mono text-xs text-emerald-400">
                ✓ storage available: 2.1 TiB
              </AnimatedSpan>
              <AnimatedSpan delay={300} className="font-mono text-xs text-emerald-400">
                ✓ accelerators detected: 8 × Ascend 910B
              </AnimatedSpan>
              <TypingAnimation delay={700} className="font-mono text-xs text-slate-300">
                $ clusteradm join --cluster-name cell1 --wait
              </TypingAnimation>
              <AnimatedSpan delay={500} className="font-mono text-xs text-sky-300">
                → host cell1 registered and waiting for approval
              </AnimatedSpan>
              <TypingAnimation delay={700} className="font-mono text-xs text-slate-300">
                $ kubectl apply -f virtualcluster.yaml
              </TypingAnimation>
              <AnimatedSpan delay={500} className="font-mono text-xs text-amber-300">
                → preflight: accepted (cpu, memory, 2 × Ascend 910B, storage)
              </AnimatedSpan>
              <AnimatedSpan delay={700} className="font-mono text-xs text-emerald-400">
                ✓ cluster child-dev is Ready
              </AnimatedSpan>
              <TypingAnimation delay={900} className="font-mono text-xs text-slate-300">
                $ kubectl --kubeconfig child-dev.yaml get nodes
              </TypingAnimation>
              <AnimatedSpan delay={400} className="font-mono text-xs text-slate-100">
                NAME     STATUS   ROLES    AGE   VERSION
              </AnimatedSpan>
              <AnimatedSpan delay={300} className="font-mono text-xs text-slate-100">
                kubelet  Ready    &lt;none&gt;   42s   v1.34.2+k3s1
              </AnimatedSpan>
            </Terminal>
          </BlurFade>
        </div>
      </div>
    </section>
  );
}
