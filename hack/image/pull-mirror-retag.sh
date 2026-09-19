#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage:
  pull-mirror-retag.sh --runtime k3s|rke2 \
    --source IMAGE --expected IMAGE [--mirror IMAGE] \
    [--platform OS/ARCH] [--max-concurrent-downloads N] [--force]

Pulls a validated mirror reference, then tags it back to the exact reference
used by the workload. It reuses an existing exact target reference unless
--force is provided. It never rewrites manifests implicitly.
USAGE
  exit 2
}

runtime=''
source_image=''
expected_image=''
mirror_image=''
platform=''
max_concurrent_downloads='8'
force='false'
while [ "$#" -gt 0 ]; do
  case "$1" in
    --runtime)
      runtime="${2:?missing runtime}"
      shift 2
      ;;
    --source)
      source_image="${2:?missing source image}"
      shift 2
      ;;
    --expected)
      expected_image="${2:?missing expected image}"
      shift 2
      ;;
    --mirror)
      mirror_image="${2:?missing mirror image}"
      shift 2
      ;;
    --platform)
      platform="${2:?missing platform}"
      shift 2
      ;;
    --max-concurrent-downloads)
      max_concurrent_downloads="${2:?missing max concurrent downloads}"
      shift 2
      ;;
    --force)
      force='true'
      shift
      ;;
    *)
      usage
      ;;
  esac
done

[ "$runtime" = k3s ] || [ "$runtime" = rke2 ] || usage
[ -n "$source_image" ] && [ -n "$expected_image" ] || usage
[ -n "$mirror_image" ] || mirror_image="$source_image"

canonical_image() {
  local image="$1"
  case "$image" in
    */*) case "${image%%/*}" in *.*|*:*|localhost) printf '%s\n' "$image" ;; *) printf 'docker.io/%s\n' "$image" ;; esac ;;
    *) printf 'docker.io/library/%s\n' "$image" ;;
  esac
}

canonical_expected="$(canonical_image "$expected_image")"

case "$runtime" in
  k3s) ctr=(/usr/local/bin/k3s ctr -n k8s.io) ;;
  rke2)
    containerd_pid="$(ps -eo pid=,args= | awk '$0 ~ /[c]ontainerd -c/ && $0 ~ /rke2/ {print $1; exit}')"
    [ -n "$containerd_pid" ] || {
      echo 'unable to find an active RKE2 containerd process' >&2
      exit 1
    }
    containerd_exe="$(readlink -f "/proc/$containerd_pid/exe")"
    containerd_config="$(ps -p "$containerd_pid" -o args= | sed -n 's/.* -c \([^ ]*\).*/\1/p')"
    [ -x "$(dirname "$containerd_exe")/ctr" ] || {
      echo "RKE2 ctr is missing beside $containerd_exe" >&2
      exit 1
    }
    [ -f "$containerd_config" ] || {
      echo "RKE2 containerd config is missing: $containerd_config" >&2
      exit 1
    }
    socket="$(sed -n 's/^[[:space:]]*address[[:space:]]*=\s*"\(.*\)"/\1/p' "$containerd_config" | head -n1)"
    [ -n "$socket" ] || {
      echo "RKE2 containerd socket is missing from $containerd_config" >&2
      exit 1
    }
    ctr=("$(dirname "$containerd_exe")/ctr" --address "$socket" --namespace k8s.io)
    ;;
esac

existing_image="$(sudo -n "${ctr[@]}" images ls | awk -v expected="$expected_image" '$1 == expected {print; found=1} END {if (!found) exit 1}' 2>/dev/null || true)"
existing_canonical="$(sudo -n "${ctr[@]}" images ls | awk -v expected="$canonical_expected" '$1 == expected {print; found=1} END {if (!found) exit 1}' 2>/dev/null || true)"
if [ "$force" != true ] && [ -n "$existing_image$existing_canonical" ]; then
  if [ -z "$existing_image" ]; then
    echo "retagging existing canonical reference: $canonical_expected -> $expected_image"
    sudo -n "${ctr[@]}" images tag "$canonical_expected" "$expected_image"
    existing_image="$(sudo -n "${ctr[@]}" images ls | awk -v expected="$expected_image" '$1 == expected {print; found=1} END {if (!found) exit 1}')"
  fi
  if [ -n "$platform" ] && ! printf '%s\n' "$existing_image" | grep -Fq "$platform"; then
    echo "exact target exists but does not advertise requested platform: $expected_image ($platform)" >&2
    exit 1
  fi
  echo "exact target already present; reusing: $expected_image"
  printf '%s\n' "$existing_image"
  exit 0
fi

pull_args=(images pull --max-concurrent-downloads "$max_concurrent_downloads")
[ -n "$platform" ] && pull_args+=(--platform "$platform")
echo "pulling validated source: $mirror_image (platform=${platform:-all}, concurrency=$max_concurrent_downloads)"
sudo -n "${ctr[@]}" "${pull_args[@]}" "$mirror_image"
if [ "$mirror_image" != "$expected_image" ]; then
  echo "retagging exact workload reference: $mirror_image -> $expected_image"
  sudo -n "${ctr[@]}" images tag "$mirror_image" "$expected_image"
fi
if [ "$canonical_expected" != "$expected_image" ]; then
  echo "retagging canonical Docker Hub reference: $expected_image -> $canonical_expected"
  sudo -n "${ctr[@]}" images tag "$expected_image" "$canonical_expected"
fi
sudo -n "${ctr[@]}" images ls | awk -v expected="$expected_image" '$1 == expected {print; found=1} END {if (!found) exit 1}'
