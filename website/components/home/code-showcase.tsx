"use client";

import { CodeBlock } from "@/components/code-block";
import { SectionHeading } from "@/components/section";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

const cellYaml = `apiVersion: kubecell.io/v1alpha1
kind: Cell
metadata:
  name: cell1
  namespace: kubecell-system
spec:
  managedClusterRef:
    name: cell1
  machineProfile:
    name: ascend-910b
    devices:
    - name: ascend
      resourceName: huawei.com/Ascend910`;

const classYaml = `apiVersion: kubecell.io/v1alpha1
kind: VirtualNodeClass
metadata:
  name: ascend-910b
spec:
  entitlement:
    workloadHard:
      requests.cpu: "4"
      limits.cpu: "4"
      requests.memory: 8Gi
      limits.memory: 8Gi
      requests.huawei.com/Ascend910: "2"
      limits.huawei.com/Ascend910: "2"
  storageClassName: topolvm-provisioner`;

const vcYaml = `apiVersion: kubecell.io/v1alpha1
kind: VirtualClusterPlan   # isomorphic dry run first
metadata:
  name: child-dev-plan
  namespace: kubecell-system
spec:
  cellRef: {name: cell1}
  classRef: {name: ascend-910b}
---
apiVersion: kubecell.io/v1alpha1
kind: VirtualCluster       # then the real thing
metadata:
  name: child-dev
  namespace: kubecell-system
spec:
  cellRef: {name: cell1}
  classRef: {name: ascend-910b}`;

export function CodeShowcase() {
  return (
    <section className="relative py-20 sm:py-24">
      <div className="container-page">
        <SectionHeading
          eyebrow="The manifests"
          title="Three small files stand between a host and a developer"
          description="A Cell describes one physical host and what it offers. A VirtualNodeClass is a quota tier. A VirtualCluster is eight lines."
        />

        <div className="mx-auto mt-12 max-w-3xl">
          <Tabs defaultValue="cell">
            <TabsList className="mx-auto flex h-auto w-fit flex-wrap justify-center gap-1 border border-white/10 bg-white/[0.03] p-1">
              <TabsTrigger value="cell">Cell</TabsTrigger>
              <TabsTrigger value="class">VirtualNodeClass</TabsTrigger>
              <TabsTrigger value="vc">Plan → VirtualCluster</TabsTrigger>
            </TabsList>
            <TabsContent value="cell" className="mt-6">
              <CodeBlock code={cellYaml} language="yaml" filename="cell.yaml" />
            </TabsContent>
            <TabsContent value="class" className="mt-6">
              <CodeBlock code={classYaml} language="yaml" filename="virtualnodeclass.yaml" />
            </TabsContent>
            <TabsContent value="vc" className="mt-6">
              <CodeBlock code={vcYaml} language="yaml" filename="virtualcluster.yaml" />
            </TabsContent>
          </Tabs>
        </div>

        <p className="mx-auto mt-6 max-w-3xl text-center text-sm leading-6 text-muted-foreground">
          Not sure a request will fit? Create the Plan first: it checks capacity without creating
          anything, tells you exactly which resource is missing, and never blocks a later change.
        </p>
      </div>
    </section>
  );
}
