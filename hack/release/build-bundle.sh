#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: build-bundle.sh --output DIR --ocm-bundle PATH --ocm-join-bundle PATH \
  --controller-image IMAGE@sha256:DIGEST --apps-suffix SUFFIX [--version VERSION] \
  [--host-profile PROFILE] [--k3k-chart PATH] [--topolvm-manifest PATH] \
  [--traefik-manifest PATH] [--device-plugin-manifest PATH]
EOF
}

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
output=
ocm_bundle=
ocm_join_bundle=
controller_image=
apps_suffix=
version=dev
host_profile=k3s-v1.36.3+k3s1-arm64-ascend
k3k_chart=
topolvm_manifest=
traefik_manifest=
device_plugin_manifest=

while [[ $# -gt 0 ]]; do
  case "$1" in
    --output) output=$2; shift 2 ;;
    --ocm-bundle) ocm_bundle=$2; shift 2 ;;
    --ocm-join-bundle) ocm_join_bundle=$2; shift 2 ;;
    --controller-image) controller_image=$2; shift 2 ;;
    --apps-suffix) apps_suffix=$2; shift 2 ;;
    --version) version=$2; shift 2 ;;
    --host-profile) host_profile=$2; shift 2 ;;
    --k3k-chart) k3k_chart=$2; shift 2 ;;
    --topolvm-manifest) topolvm_manifest=$2; shift 2 ;;
    --traefik-manifest) traefik_manifest=$2; shift 2 ;;
    --device-plugin-manifest) device_plugin_manifest=$2; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

[[ -n "$output" && -n "$ocm_bundle" && -n "$ocm_join_bundle" && -n "$controller_image" && -n "$apps_suffix" ]] || { usage >&2; exit 2; }
[[ "$controller_image" == *@sha256:* ]] || { echo "controller image must use @sha256:digest" >&2; exit 2; }

if [[ -e "$output" ]]; then
  if [[ ! -d "$output" || -n "$(find "$output" -mindepth 1 -maxdepth 1 -print -quit)" ]]; then
    echo "refusing to overwrite non-empty release output: $output" >&2
    exit 2
  fi
fi
mkdir -p "$output/ocm" "$output/crds" "$output/charts"
copy_artifact() {
  local source=$1 destination=$2
  if [[ -d "$source" ]]; then
    mkdir -p "$destination"
    cp -R "$source"/. "$destination"/
  else
    mkdir -p "$(dirname "$destination")"
    cp "$source" "$destination"
  fi
}
copy_artifact "$ocm_bundle" "$output/ocm/bundle"
copy_artifact "$ocm_join_bundle" "$output/ocm/join"
copy_artifact "$root/config/crd/bases" "$output/crds"
copy_artifact "$root/charts/kubecell-management" "$output/charts/kubecell-management"
copy_artifact "$root/charts/kubecell-host" "$output/charts/kubecell-host"

# Optional Kubernetes-side baseline artifacts (vendored chart/manifest): when provided,
# they are vendored into the bundle and recorded with a digest.
# When omitted, the manifest stays empty and the installer only validates set values (same B2 coverage).
baseline_lines=""
baseline_digests=""
add_baseline() {
  local field=$1 source=$2 destination=$3
  if [[ -z "$source" ]]; then return 0; fi
  copy_artifact "$source" "$output/$destination"
  baseline_lines="${baseline_lines}${field}: ${destination}
"
  baseline_digests="${baseline_digests}  ${destination}: $(digest "$output/$destination")
"
}
add_baseline k3kChartPath "$k3k_chart" baseline/k3k-chart
add_baseline topolvmPath "$topolvm_manifest" baseline/topolvm
add_baseline traefikPath "$traefik_manifest" baseline/traefik
add_baseline devicePluginPath "$device_plugin_manifest" baseline/device-plugin

digest() { (cd "$root" && go run ./cmd/kubecell-installer digest "$1"); }

cat > "$output/compatibility.yaml" <<EOF
kubecellVersion: "$version"
k3kVersion: v1.2.0
childK3sVersion: v1.34.2+k3s1
ocmVersion: v1.3.1
ocmBundleDigest: $(digest "$output/ocm/bundle")
ocmBundlePath: ocm/bundle
ocmJoinBundlePath: ocm/join
ocmJoinCommand:
- clusteradm
- join
- --bundle
- '{{OCMJoinBundlePath}}'
kubecellCRDPath: crds
managementChartPath: charts/kubecell-management
hostChartPath: charts/kubecell-host
controllerImage: "$controller_image"
hostProfiles:
- "$host_profile"
appsSuffix: "$apps_suffix"
hostBaselineCommands:
- 'kubectl label node {{HOST_NODE_NAME}} hardware.kubecell.io/profile={{HOST_PROFILE}} --overwrite'
- 'curl -sfL {{K3S_INSTALLER_URL}} | INSTALL_K3S_VERSION={{HOST_K3S_VERSION}} sh -s - {{K3S_INSTALL_ARGS}}'
${baseline_lines}artifactDigests:
  ocm/bundle: $(digest "$output/ocm/bundle")
  ocm/join: $(digest "$output/ocm/join")
  crds: $(digest "$output/crds")
  charts/kubecell-management: $(digest "$output/charts/kubecell-management")
  charts/kubecell-host: $(digest "$output/charts/kubecell-host")
${baseline_digests}
EOF
echo "release bundle written to $output"
