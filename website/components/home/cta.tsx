"use client";

import { Backlight } from "@/components/magicui/backlight";
import { Meteors } from "@/components/magicui/meteors";
import { ShimmerLink } from "@/components/shimmer-link";
import { Button } from "@/components/ui/button";
import { siteConfig } from "@/lib/site";

export function CallToAction() {
  return (
    <section className="relative py-20 sm:py-28">
      <div className="container-page">
        <div className="relative mx-auto max-w-4xl overflow-hidden rounded-3xl border border-white/10 bg-[#060a13] px-6 py-16 text-center sm:px-14">
          <Meteors number={22} className="opacity-70" />
          <div className="pointer-events-none absolute -top-24 left-1/2 h-64 w-[36rem] -translate-x-1/2 rounded-full bg-sky-500/15 blur-[100px]" />
          <div className="relative">
            <Backlight blur={14} className="mx-auto">
              <h2 className="text-balance text-3xl font-semibold tracking-tight text-foreground sm:text-4xl">
                Give every developer a real cluster
              </h2>
            </Backlight>
            <p className="mx-auto mt-4 max-w-2xl text-pretty text-base leading-7 text-muted-foreground">
              Start with the read-only preflight. Nothing is created until you explicitly confirm, so
              the first command you run is always safe.
            </p>
            <div className="mt-8 flex flex-col items-center justify-center gap-3 sm:flex-row">
              <ShimmerLink href="/quickstart/" className="h-11 px-6 text-sm font-medium">
                Read the quickstart
              </ShimmerLink>
              <Button variant="outline" size="lg" asChild>
                <a href={siteConfig.github} target="_blank" rel="noreferrer">
                  Browse the source
                </a>
              </Button>
            </div>
            <p className="mt-6 font-mono text-xs text-muted-foreground">
              go run ./cmd/kubecell-installer doctor management --bundle ./kubecell-release
            </p>
          </div>
        </div>
      </div>
    </section>
  );
}
