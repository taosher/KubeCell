import type { Metadata, Viewport } from "next";

import "@fontsource-variable/inter";
import "@fontsource-variable/jetbrains-mono";
import "./globals.css";

import { JsonLd } from "@/components/json-ld";
import { ScrollProgress } from "@/components/magicui/scroll-progress";
import { SiteFooter } from "@/components/site-footer";
import { SiteHeader } from "@/components/site-header";
import { absoluteUrl, siteConfig } from "@/lib/site";

export const metadata: Metadata = {
  metadataBase: new URL(siteConfig.url),
  title: {
    default: siteConfig.title,
    template: "%s | KubeCell",
  },
  description: siteConfig.description,
  keywords: [...siteConfig.keywords],
  applicationName: siteConfig.name,
  authors: [{ name: "KubeCell contributors", url: siteConfig.github }],
  creator: "KubeCell",
  publisher: "KubeCell",
  category: "technology",
  alternates: {
    canonical: "/",
  },
  openGraph: {
    type: "website",
    locale: "en_US",
    url: absoluteUrl("/"),
    siteName: siteConfig.name,
    title: siteConfig.title,
    description: siteConfig.description,
    images: [
      {
        url: "/og.png",
        width: 1200,
        height: 630,
        alt: "KubeCell — isolated Kubernetes child clusters on shared bare metal",
      },
    ],
  },
  twitter: {
    card: "summary_large_image",
    title: siteConfig.title,
    description: siteConfig.shortDescription,
    images: ["/og.png"],
  },
  robots: {
    index: true,
    follow: true,
    googleBot: {
      index: true,
      follow: true,
      "max-image-preview": "large",
      "max-snippet": -1,
      "max-video-preview": -1,
    },
  },
  icons: {
    icon: [{ url: "/kubecell-logo.svg", type: "image/svg+xml" }],
    shortcut: ["/kubecell-logo.svg"],
    apple: [{ url: "/apple-icon.png", sizes: "180x180" }],
  },
  formatDetection: {
    telephone: false,
  },
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  colorScheme: "dark",
  themeColor: "#04060d",
};

const organizationLd = {
  "@context": "https://schema.org",
  "@type": "Organization",
  name: "KubeCell",
  url: siteConfig.url,
  logo: absoluteUrl("/kubecell-logo.svg"),
  sameAs: [siteConfig.github],
  description: siteConfig.description,
};

const websiteLd = {
  "@context": "https://schema.org",
  "@type": "WebSite",
  name: "KubeCell",
  url: siteConfig.url,
  description: siteConfig.description,
  publisher: { "@type": "Organization", name: "KubeCell" },
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en" className="dark">
      <body className="min-h-screen antialiased">
        <JsonLd data={[organizationLd, websiteLd]} />
        <ScrollProgress className="h-0.5 bg-gradient-to-r from-sky-400 via-violet-400 to-sky-400" />
        <a
          href="#main"
          className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-[100] focus:rounded-md focus:bg-primary focus:px-4 focus:py-2 focus:text-primary-foreground"
        >
          Skip to content
        </a>
        <SiteHeader />
        <main id="main">{children}</main>
        <SiteFooter />
      </body>
    </html>
  );
}
