import { writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import { docsFlat } from "../lib/docs-nav.ts";
import { siteConfig } from "../lib/site.ts";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");

type Route = {
  path: string;
  priority: number;
  changeFrequency: "weekly" | "monthly";
};

const staticRoutes: Route[] = [
  { path: "/", priority: 1, changeFrequency: "weekly" },
  { path: "/quickstart/", priority: 0.9, changeFrequency: "monthly" },
  { path: "/use-cases/", priority: 0.8, changeFrequency: "monthly" },
  { path: "/architecture/", priority: 0.8, changeFrequency: "monthly" },
];

const docRoutes: Route[] = docsFlat.map((doc, index) => ({
  path: doc.href,
  priority: index === 0 ? 0.9 : 0.7,
  changeFrequency: "monthly",
}));

const routes = [...staticRoutes, ...docRoutes];
const lastmod = new Date().toISOString().slice(0, 10);

const sitemap = [
  '<?xml version="1.0" encoding="UTF-8"?>',
  '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">',
  ...routes.map((route) =>
    [
      "  <url>",
      `    <loc>${new URL(route.path, siteConfig.url).toString()}</loc>`,
      `    <lastmod>${lastmod}</lastmod>`,
      `    <changefreq>${route.changeFrequency}</changefreq>`,
      `    <priority>${route.priority.toFixed(1)}</priority>`,
      "  </url>",
    ].join("\n")
  ),
  "</urlset>",
  "",
].join("\n");

const robots = [
  "# https://www.robotstxt.org/robotstxt.html",
  "User-agent: *",
  "Allow: /",
  "",
  `Sitemap: ${new URL("/sitemap.xml", siteConfig.url).toString()}`,
  `Host: ${siteConfig.url}`,
  "",
].join("\n");

const manifest = {
  name: siteConfig.title,
  short_name: siteConfig.name,
  description: siteConfig.shortDescription,
  start_url: "/",
  display: "standalone",
  background_color: "#04060d",
  theme_color: "#04060d",
  icons: [
    {
      src: "/kubecell-logo.svg",
      sizes: "any",
      type: "image/svg+xml",
      purpose: "any",
    },
    {
      src: "/apple-icon.png",
      sizes: "180x180",
      type: "image/png",
    },
  ],
};

const outputs: Array<[string, string]> = [
  ["app/sitemap.xml", sitemap],
  ["app/robots.txt", robots],
  ["app/manifest.webmanifest", `${JSON.stringify(manifest, null, 2)}\n`],
];

for (const [relativePath, content] of outputs) {
  const target = join(root, relativePath);
  writeFileSync(target, content, "utf-8");
  console.log(`[seo] wrote ${relativePath}`);
}

console.log(`[seo] ${routes.length} sitemap entries for ${siteConfig.url}`);
