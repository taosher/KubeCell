import Link from "next/link";

import { JsonLd } from "@/components/json-ld";
import { Prose } from "@/components/docs/callout";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { faqItems } from "@/lib/faq";
import { buildMetadata } from "@/lib/metadata";
import { absoluteUrl } from "@/lib/site";

export const metadata = buildMetadata({
  title: "FAQ",
  description:
    "Frequently asked questions about KubeCell: what a VirtualCluster is, accelerator support, isolation guarantees, logical nodes, deletion semantics, and production readiness.",
  path: "/docs/faq/",
  keywords: ["KubeCell FAQ", "KubeCell limitations", "virtual cluster questions"],
});

const faqLd = {
  "@context": "https://schema.org",
  "@type": "FAQPage",
  mainEntity: faqItems.map((item) => ({
    "@type": "Question",
    name: item.question,
    acceptedAnswer: {
      "@type": "Answer",
      text: item.answer,
    },
  })),
  url: absoluteUrl("/docs/faq/"),
};

export default function FaqPage() {
  return (
    <>
      <JsonLd data={faqLd} />
      <Prose>
        <p className="font-mono text-xs uppercase tracking-[0.2em] text-sky-400">Guides</p>
        <h1 className="mt-3 text-3xl font-semibold tracking-tight text-foreground sm:text-4xl">
          Frequently asked questions
        </h1>
        <p className="mt-5 text-base leading-7 text-muted-foreground">
          Short answers to the questions that come up in every design review.
        </p>

        <div className="not-prose mt-8">
          <Accordion type="single" collapsible className="w-full">
            {faqItems.map((item, index) => (
              <AccordionItem key={item.question} value={`item-${index}`}>
                <AccordionTrigger className="text-left text-[15px] font-medium hover:text-sky-300">
                  {item.question}
                </AccordionTrigger>
                <AccordionContent className="text-sm leading-6 text-muted-foreground">
                  {item.answer}
                </AccordionContent>
              </AccordionItem>
            ))}
          </Accordion>
        </div>

        <p className="mt-10">
          Still stuck? The <Link href="/docs/troubleshooting/">troubleshooting guide</Link> covers
          symptom-first diagnosis, and the{" "}
          <a href="https://github.com/taosher/KubeCell/blob/main/technical-design.md">
            technical design
          </a>{" "}
          is the authoritative answer for anything not listed here.
        </p>
      </Prose>
    </>
  );
}
