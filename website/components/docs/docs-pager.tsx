"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { ArrowLeft, ArrowRight } from "lucide-react";

import { docNeighbors } from "@/lib/docs-nav";

export function DocsPager() {
  const pathname = usePathname();
  const { previous, next } = docNeighbors(pathname);

  if (!previous && !next) return null;

  return (
    <div className="mt-16 grid gap-4 border-t border-white/5 pt-8 sm:grid-cols-2">
      {previous ? (
        <Link
          href={previous.href}
          className="group rounded-xl border border-white/10 bg-white/[0.02] p-4 transition-colors hover:border-sky-400/30"
        >
          <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
            <ArrowLeft className="h-3.5 w-3.5 transition-transform group-hover:-translate-x-0.5" />
            Previous
          </span>
          <p className="mt-1.5 text-sm font-medium text-foreground">{previous.title}</p>
        </Link>
      ) : (
        <div />
      )}
      {next ? (
        <Link
          href={next.href}
          className="group rounded-xl border border-white/10 bg-white/[0.02] p-4 text-right transition-colors hover:border-sky-400/30 sm:col-start-2"
        >
          <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
            Next
            <ArrowRight className="h-3.5 w-3.5 transition-transform group-hover:translate-x-0.5" />
          </span>
          <p className="mt-1.5 text-sm font-medium text-foreground">{next.title}</p>
        </Link>
      ) : null}
    </div>
  );
}
