"use client";

import Link from "next/link";
import { ArrowRight, Github, Terminal as TerminalIcon } from "lucide-react";

import { BlurFade } from "@/components/magicui/blur-fade";
import { AnimatedGridPattern } from "@/components/magicui/animated-grid-pattern";
import { AnimatedGradientText } from "@/components/magicui/animated-gradient-text";
import { AnimatedShinyText } from "@/components/magicui/animated-shiny-text";
import { BorderBeam } from "@/components/magicui/border-beam";
import { NumberTicker } from "@/components/magicui/number-ticker";
import { WordRotate } from "@/components/magicui/word-rotate";
import { ShimmerLink } from "@/components/shimmer-link";
import { CodeBlock } from "@/components/code-block";
import { Button } from "@/components/ui/button";
import { siteConfig } from "@/lib/site";
import { cn } from "@/lib/utils";

const heroManifest = `apiVersion: kubecell.io/v1alpha1
kind: VirtualCluster
metadata:
  name: child-dev
  namespace: kubecell-system
spec:
  cellRef: {name: cell1}
  classRef: {name: ascend-910b}`;

const heroStats = [
  { value: 0, suffix: "", label: "VMs to manage" },
  { value: 8, suffix: "", label: "lines to create a cluster" },
  { value: 20, suffix: " min", label: "from apply to first Ready" },
  { value: 100, suffix: "%", label: "standard kubectl, Helm, and CI" },
];

export function Hero() {
  return (
    <section className="relative overflow-hidden pb-16 pt-16 sm:pb-24 sm:pt-24">
      <AnimatedGridPattern
        numSquares={36}
        maxOpacity={0.12}
        duration={4}
        repeatDelay={0.6}
        className={cn(
          "absolute inset-0 -z-10 h-[46rem] w-full",
          "[mask-image:radial-gradient(60%_60%_at_50%_20%,black,transparent)]"
        )}
      />
      <div className="pointer-events-none absolute -top-32 left-1/2 -z-10 h-[28rem] w-[52rem] -translate-x-1/2 rounded-full bg-sky-500/10 blur-[120px]" />
      <div className="pointer-events-none absolute inset-x-0 bottom-0 -z-10 h-40 bg-gradient-to-b from-transparent to-background" />

      <div className="container-page relative">
        <div className="mx-auto max-w-3xl text-center">
          <BlurFade inView>
            <Link
              href="/docs/"
              className="group inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/[0.03] px-3.5 py-1.5 text-xs text-muted-foreground transition-colors hover:border-sky-400/40 hover:text-foreground"
            >
              <span className="inline-flex h-1.5 w-1.5 rounded-full bg-emerald-400" />
              <AnimatedShinyText className="text-xs">
                Open source · pre-release · not production ready yet
              </AnimatedShinyText>
              <ArrowRight className="h-3 w-3 transition-transform group-hover:translate-x-0.5" />
            </Link>
          </BlurFade>

          <BlurFade delay={0.08} inView>
            <h1 className="mt-6 text-pretty text-4xl font-semibold leading-[1.12] tracking-tight text-foreground sm:text-5xl">
              Give every developer{" "}
              <br className="hidden sm:inline" />
              <AnimatedGradientText colorFrom="#7fb6ff" colorTo="#c4b5fd" speed={1}>
                their own Kubernetes cluster
              </AnimatedGradientText>
            </h1>
          </BlurFade>

          <BlurFade delay={0.12} inView>
            <div className="mt-5 flex flex-wrap items-center justify-center gap-2 text-sm text-muted-foreground">
              <span>Built for</span>
              <WordRotate
                className="text-sm font-medium text-sky-300"
                words={[
                  "AI research teams",
                  "platform engineering",
                  "training labs",
                  "CI pipelines",
                  "edge sites",
                ]}
              />
            </div>
          </BlurFade>

          <BlurFade delay={0.16} inView>
            <p className="mx-auto mt-6 max-w-2xl text-pretty text-base leading-7 text-muted-foreground sm:text-lg">
              Each developer gets an isolated cluster on the hardware you already own: standard{" "}
              <span className="text-foreground/90">kubectl</span>, a real control plane, persistent
              storage, and hard CPU, memory, and GPU quotas. No VM per person, no shared namespace
              everyone has to trust.
            </p>
          </BlurFade>

          <BlurFade delay={0.24} inView>
            <div className="mt-8 flex flex-col items-center justify-center gap-3 sm:flex-row">
              <ShimmerLink href="/quickstart/" className="h-11 px-6 text-sm font-medium shadow-2xl">
                <TerminalIcon className="h-4 w-4" />
                Start the quickstart
              </ShimmerLink>
              <Button variant="outline" size="lg" asChild>
                <a href={siteConfig.github} target="_blank" rel="noreferrer">
                  <Github className="h-4 w-4" />
                  View on GitHub
                </a>
              </Button>
            </div>
          </BlurFade>
        </div>

        <BlurFade delay={0.32} inView>
          <div className="relative mx-auto mt-14 max-w-2xl">
            <div className="absolute -inset-x-6 -inset-y-4 -z-10 rounded-3xl bg-gradient-to-r from-sky-500/10 via-transparent to-violet-500/10 blur-2xl" />
            <CodeBlock code={heroManifest} language="yaml" filename="virtualcluster.yaml" />
            <BorderBeam
              size={220}
              duration={10}
              colorFrom="#38bdf8"
              colorTo="#8b5cf6"
              borderWidth={1}
              className="rounded-xl"
            />
          </div>
        </BlurFade>

        <BlurFade delay={0.4} inView>
          <dl className="mx-auto mt-14 grid max-w-4xl grid-cols-2 gap-x-6 gap-y-8 sm:grid-cols-4">
            {heroStats.map((stat) => (
              <div key={stat.label} className="text-center">
                <dt className="sr-only">{stat.label}</dt>
                <dd className="text-3xl font-semibold tracking-tight text-foreground sm:text-4xl">
                  <NumberTicker value={stat.value} className="text-foreground" />
                  {stat.suffix}
                </dd>
                <p className="mt-1.5 text-xs leading-5 text-muted-foreground">{stat.label}</p>
              </div>
            ))}
          </dl>
        </BlurFade>
      </div>
    </section>
  );
}
