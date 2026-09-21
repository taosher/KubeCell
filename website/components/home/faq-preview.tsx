"use client";

import Link from "next/link";

import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { SectionHeading } from "@/components/section";
import { faqItems } from "@/lib/faq";

export function FaqPreview() {
  return (
    <section className="relative py-20 sm:py-24">
      <div className="container-page">
        <div className="grid gap-10 lg:grid-cols-[0.9fr_1.1fr] lg:gap-16">
          <SectionHeading
            align="left"
            eyebrow="FAQ"
            title="Questions engineers ask first"
            description="Design trade-offs worth knowing before you install anything."
          />
          <div>
            <Accordion type="single" collapsible className="w-full">
              {faqItems.slice(0, 5).map((item) => (
                <AccordionItem key={item.question} value={item.question}>
                  <AccordionTrigger className="text-left text-[15px] font-medium hover:text-sky-300">
                    {item.question}
                  </AccordionTrigger>
                  <AccordionContent className="text-sm leading-6 text-muted-foreground">
                    {item.answer}
                  </AccordionContent>
                </AccordionItem>
              ))}
            </Accordion>
            <p className="mt-6 text-sm text-muted-foreground">
              More questions answered in the{" "}
              <Link href="/docs/faq/" className="font-medium text-sky-400 hover:text-sky-300">
                full FAQ
              </Link>
              .
            </p>
          </div>
        </div>
      </div>
    </section>
  );
}
