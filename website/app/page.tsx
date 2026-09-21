import Link from "next/link";
import { ArrowRight } from "lucide-react";

import { ArchitectureBeam } from "@/components/home/architecture-beam";
import { CallToAction } from "@/components/home/cta";
import { CodeShowcase } from "@/components/home/code-showcase";
import { Comparison } from "@/components/home/comparison";
import { FaqPreview } from "@/components/home/faq-preview";
import { FeatureBento } from "@/components/home/feature-bento";
import { Hero } from "@/components/home/hero";
import { IsolationGrid } from "@/components/home/isolation";
import { StackMarquee } from "@/components/home/stack-marquee";
import { UseCasePreview } from "@/components/home/use-cases-preview";
import { Workflow } from "@/components/home/workflow";
import { JsonLd } from "@/components/json-ld";
import { Section, SectionHeading } from "@/components/section";
import { pinnedStack } from "@/lib/content";
import { absoluteUrl, siteConfig } from "@/lib/site";

const softwareLd = {
  "@context": "https://schema.org",
  "@type": "SoftwareSourceCode",
  name: "KubeCell",
  description: siteConfig.description,
  codeRepository: siteConfig.github,
  programmingLanguage: "Go",
  runtimePlatform: "Kubernetes",
  applicationCategory: "DeveloperApplication",
  license: `${siteConfig.github}/blob/main/LICENSE`,
  author: { "@type": "Organization", name: "KubeCell" },
  url: absoluteUrl("/"),
};

export default function HomePage() {
  return (
    <>
      <JsonLd data={softwareLd} />
      <Hero />
      <StackMarquee />
      <FeatureBento />
      <Workflow />

      <Section className="border-y border-white/5 bg-white/[0.015]">
        <SectionHeading
          eyebrow="Architecture"
          title="One management plane, many isolated clusters"
          description="You apply manifests to a single management cluster. KubeCell turns them into changes on the host, then hands back an endpoint and a kubeconfig."
        />
        <div className="mt-12">
          <ArchitectureBeam />
        </div>
        <div className="mt-8 text-center">
          <Link
            href="/architecture/"
            className="inline-flex items-center gap-1.5 text-sm font-medium text-sky-400 transition-colors hover:text-sky-300"
          >
            Read the technical principles
            <ArrowRight className="h-3.5 w-3.5" />
          </Link>
        </div>
      </Section>

      <IsolationGrid />
      <Comparison />
      <CodeShowcase />

      <Section className="border-y border-white/5 bg-white/[0.015]">
        <SectionHeading
          eyebrow="Tested versions"
          title="Versions that are validated together"
          description="KubeCell pins and verifies the components each release supports, and reports any drift in resource status — so upgrades are a decision, not a surprise."
        />
        <div className="mt-12 overflow-hidden rounded-2xl border border-white/10">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[640px] border-collapse text-left text-sm">
              <thead>
                <tr className="bg-white/[0.03]">
                  <th className="px-5 py-3.5 font-medium text-muted-foreground">Component</th>
                  <th className="px-5 py-3.5 font-medium text-muted-foreground">Version</th>
                  <th className="px-5 py-3.5 font-medium text-muted-foreground">What it does for you</th>
                </tr>
              </thead>
              <tbody>
                {pinnedStack.map((row) => (
                  <tr key={row.component} className="border-t border-white/5">
                    <td className="px-5 py-3.5 font-medium text-foreground">{row.component}</td>
                    <td className="px-5 py-3.5 font-mono text-xs text-sky-300">{row.version}</td>
                    <td className="px-5 py-3.5 text-muted-foreground">{row.note}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </Section>

      <UseCasePreview />
      <FaqPreview />
      <CallToAction />
    </>
  );
}
