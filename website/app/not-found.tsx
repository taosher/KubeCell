import Link from "next/link";

import { Button } from "@/components/ui/button";

export default function NotFound() {
  return (
    <div className="relative overflow-hidden">
      <div className="pointer-events-none absolute inset-0 bg-grid-faint opacity-50" />
      <div className="container-page relative flex min-h-[60vh] flex-col items-center justify-center py-24 text-center">
        <p className="font-mono text-xs uppercase tracking-[0.24em] text-sky-400">404</p>
        <h1 className="mt-4 text-3xl font-semibold tracking-tight text-foreground sm:text-4xl">
          This cluster does not exist
        </h1>
        <p className="mt-4 max-w-lg text-pretty text-base leading-7 text-muted-foreground">
          The page you requested is not in the desired state and cannot be reconciled. Try the
          documentation index or head back home.
        </p>
        <div className="mt-8 flex flex-wrap items-center justify-center gap-3">
          <Button asChild>
            <Link href="/">Back to home</Link>
          </Button>
          <Button variant="outline" asChild>
            <Link href="/docs/">Browse docs</Link>
          </Button>
        </div>
      </div>
    </div>
  );
}
