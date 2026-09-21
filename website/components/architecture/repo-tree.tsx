"use client";

import { Tree, type TreeViewElement } from "@/components/magicui/file-tree";

const elements: TreeViewElement[] = [
  {
    id: "api",
    name: "api/",
    type: "folder",
    children: [{ id: "api-types", name: "CRD types and validation", type: "file" }],
  },
  {
    id: "cmd",
    name: "cmd/",
    type: "folder",
    children: [
      { id: "cmd-management", name: "management controller", type: "file" },
      { id: "cmd-webhook", name: "host webhook", type: "file" },
      { id: "cmd-installer", name: "kubecell-installer", type: "file" },
    ],
  },
  {
    id: "internal",
    name: "internal/",
    type: "folder",
    children: [
      { id: "internal-controllers", name: "controllers and rendering", type: "file" },
      { id: "internal-admission", name: "admission", type: "file" },
      { id: "internal-platform", name: "platform constants", type: "file" },
    ],
  },
  {
    id: "charts",
    name: "charts/",
    type: "folder",
    children: [
      { id: "charts-management", name: "kubecell-management", type: "file" },
      { id: "charts-host", name: "kubecell-host", type: "file" },
    ],
  },
  {
    id: "config",
    name: "config/",
    type: "folder",
    children: [{ id: "config-kubebuilder", name: "CRDs, RBAC, webhook, manager", type: "file" }],
  },
  {
    id: "third-party",
    name: "third-party/",
    type: "folder",
    children: [{ id: "third-party-ocm", name: "pinned upstream versions (OCM)", type: "file" }],
  },
  {
    id: "hack",
    name: "hack/",
    type: "folder",
    children: [{ id: "hack-image", name: "image and release helpers", type: "file" }],
  },
  {
    id: "test",
    name: "test/",
    type: "folder",
    children: [{ id: "test-envtest", name: "envtest, chart contracts, e2e", type: "file" }],
  },
  {
    id: "website",
    name: "website/",
    type: "folder",
    children: [{ id: "website-site", name: "this site and documentation", type: "file" }],
  },
  { id: "design", name: "technical-design.md", type: "file" },
];

export function RepoTree() {
  return (
    <div className="relative h-[26rem] overflow-hidden rounded-2xl border border-white/10 bg-[#070b14] p-4">
      <Tree
        elements={elements}
        initialExpandedItems={["api", "cmd", "internal", "charts"]}
        className="[&_button]:text-slate-300 [&_button:hover]:bg-white/5"
      />
    </div>
  );
}
