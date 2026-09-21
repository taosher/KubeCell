import { AlertTriangle, Clock, Database, FileWarning, ServerCrash, ShieldAlert } from "lucide-react";

import { BorderBeam } from "@/components/magicui/border-beam";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

const failures = [
  {
    icon: Clock,
    title: "Host lease goes stale",
    symptom: "Cell reports Degraded even though OCM still says Available.",
    action:
      "Trust the lease, not the condition. Check klusterlet health and node time skew before creating new child clusters.",
  },
  {
    icon: Database,
    title: "Storage capacity unknown",
    symptom: "storageInventory.freeCapacityKnown is false, or the observation is old.",
    action:
      "No feasibility conclusion is trustworthy. Fix TopoLVM/lvmd reporting and wait for a fresh Cell observation.",
  },
  {
    icon: ServerCrash,
    title: "Work not Applied",
    symptom: "A VirtualCluster stays in Provisioning and never becomes Ready.",
    action:
      "Walk the chain: the work status in the hub, the work-agent on the host, then the target namespace quota and any admission rejections.",
  },
  {
    icon: FileWarning,
    title: "Child API unreachable",
    symptom: "The kubeconfig exists but connections time out.",
    action:
      "Compare status.endpoint with the published Secret, then check K3k server Pods, the NodePort Service, and TopoLVM PVCs on the host.",
  },
  {
    icon: AlertTriangle,
    title: "Capability mismatch",
    symptom: "A Cell declares more accelerator capacity than the host actually allocates.",
    action:
      "Observed allocatable always wins. Align the machine profile declaration with the Device Plugin registration, then re-observe.",
  },
  {
    icon: ShieldAlert,
    title: "Tenant workload rejected",
    symptom: "Admission denies a child Pod for privileged access or hostPath volumes.",
    action:
      "This is expected. Use the rejection hint to pick a supported replacement, such as a PVC on the TopoLVM storage class.",
  },
];

export function FailureModes() {
  return (
    <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
      {failures.map((failure, index) => (
        <Card
          key={failure.title}
          className="relative h-full overflow-hidden border-white/10 bg-white/[0.02]"
        >
          {index === 1 ? (
            <BorderBeam size={140} duration={9} colorFrom="#f59e0b" colorTo="#ef4444" />
          ) : null}
          <CardHeader>
            <span className="inline-flex h-10 w-10 items-center justify-center rounded-xl border border-amber-400/20 bg-amber-500/10 text-amber-300">
              <failure.icon className="h-5 w-5" />
            </span>
            <CardTitle className="mt-3 text-base">{failure.title}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3 text-sm leading-6">
            <p className="text-muted-foreground">
              <span className="font-medium text-foreground/90">Symptom: </span>
              {failure.symptom}
            </p>
            <p className="text-muted-foreground">
              <span className="font-medium text-foreground/90">Action: </span>
              {failure.action}
            </p>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
