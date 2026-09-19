#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage:
  distribute-oci-archive.sh --archive FILE --image IMAGE \
    [--platform OS/ARCH] [--force] \
    --target USER@HOST:k3s [--target USER@HOST:rke2 ...]

The archive must contain IMAGE under the exact reference expected by the
workload. Each target is imported in the runtime's k8s.io namespace and then
checked with the runtime's image listing command.
USAGE
  exit 2
}

archive=''
image=''
platform=''
force='false'
declare -a targets=()

while [ "$#" -gt 0 ]; do
  case "$1" in
    --archive)
      [ "$#" -ge 2 ] || usage
      archive="$2"
      shift 2
      ;;
    --image)
      [ "$#" -ge 2 ] || usage
      image="$2"
      shift 2
      ;;
    --platform)
      [ "$#" -ge 2 ] || usage
      platform="$2"
      shift 2
      ;;
    --force)
      force='true'
      shift
      ;;
    --target)
      [ "$#" -ge 2 ] || usage
      targets+=("$2")
      shift 2
      ;;
    *)
      usage
      ;;
  esac
done

[ -n "$archive" ] && [ -f "$archive" ] || usage
[ -n "$image" ] || usage
[ "${#targets[@]}" -gt 0 ] || usage

archive="$(cd "$(dirname "$archive")" && pwd)/$(basename "$archive")"
archive_sha256="$(sha256sum "$archive" | awk '{print $1}')"

canonical_image() {
  local image="$1"
  case "$image" in
    */*) case "${image%%/*}" in *.*|*:*|localhost) printf '%s\n' "$image" ;; *) printf 'docker.io/%s\n' "$image" ;; esac ;;
    *) printf 'docker.io/library/%s\n' "$image" ;;
  esac
}

canonical_image_ref="$(canonical_image "$image")"

import_target() {
  local target="$1"
  local host runtime remote_archive
  case "$target" in
    *:k3s)
      host="${target%:k3s}"
      runtime=k3s
      ctr='/usr/local/bin/k3s ctr -n k8s.io'
      ;;
    *:rke2)
      host="${target%:rke2}"
      runtime=rke2
      ;;
    *)
      echo "invalid target '$target'; suffix must be :k3s or :rke2" >&2
      return 2
      ;;
  esac

  remote_archive="/tmp/kubecell-image-${archive_sha256}.oci"
if [ "$force" != true ] && ssh -o BatchMode=yes -o ConnectTimeout=15 "$host" \
    "sudo -n bash -s -- '$image' '$canonical_image_ref' '$runtime' '$platform'" <<'REMOTE_CHECK'
set -euo pipefail
expected_image="$1"
canonical_image="$2"
runtime="$3"
platform="$4"

if [ "$runtime" = k3s ]; then
  ctr=(/usr/local/bin/k3s ctr -n k8s.io)
else
  containerd_pid="$(ps -eo pid=,args= | awk '$0 ~ /[c]ontainerd -c/ && $0 ~ /rke2/ {print $1; exit}')"
  [ -n "$containerd_pid" ] || exit 1
  containerd_exe="$(readlink -f "/proc/$containerd_pid/exe")"
  containerd_config="$(ps -p "$containerd_pid" -o args= | sed -n 's/.* -c \([^ ]*\).*/\1/p')"
  socket="$(sed -n 's/^[[:space:]]*address[[:space:]]*=\s*"\(.*\)"/\1/p' "$containerd_config" | head -n1)"
  [ -x "$(dirname "$containerd_exe")/ctr" ] && [ -n "$socket" ] || exit 1
  ctr=("$(dirname "$containerd_exe")/ctr" --address "$socket" --namespace k8s.io)
fi

line="$("${ctr[@]}" images ls | awk -v expected="$expected_image" '$1 == expected {print; found=1} END {if (!found) exit 1}' 2>/dev/null || true)"
canonical_line="$("${ctr[@]}" images ls | awk -v expected="$canonical_image" '$1 == expected {print; found=1} END {if (!found) exit 1}' 2>/dev/null || true)"
[ -n "$line$canonical_line" ] || exit 1
if [ -z "$line" ]; then
  "${ctr[@]}" images tag "$canonical_image" "$expected_image"
  line="$("${ctr[@]}" images ls | awk -v expected="$expected_image" '$1 == expected {print; found=1} END {if (!found) exit 1}')"
fi
if [ -n "$platform" ] && ! printf '%s\n' "$line" | grep -Fq "$platform"; then
  exit 1
fi
printf '%s\n' "$line"
REMOTE_CHECK
  then
    echo "reuse existing image on $host ($runtime): $image"
    return 0
  fi

  echo "transfer $archive -> $host:$remote_archive ($runtime)"
  scp -q -o BatchMode=yes -o ConnectTimeout=15 "$archive" "$host:$remote_archive"
  echo "import and verify $image on $host ($runtime)"
  ssh -o BatchMode=yes -o ConnectTimeout=15 "$host" \
    "sudo -n bash -s -- '$remote_archive' '$archive_sha256' '$image' '$canonical_image_ref' '$runtime' '$platform'" <<'REMOTE'
set -euo pipefail

remote_archive="$1"
expected_sha256="$2"
expected_image="$3"
canonical_image="$4"
runtime="$5"
platform="$6"

remote_sha256="$(sha256sum "$remote_archive" | awk '{print $1}')"
test "$remote_sha256" = "$expected_sha256"

if [ "$runtime" = k3s ]; then
  ctr=(/usr/local/bin/k3s ctr -n k8s.io)
else
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
  socket="$(sed -n 's/^[[:space:]]*address[[:space:]]*=[[:space:]]*"\(.*\)"/\1/p' "$containerd_config" | head -n1)"
  [ -n "$socket" ] || {
    echo "RKE2 containerd socket is missing from $containerd_config" >&2
    exit 1
  }
  ctr=("$(dirname "$containerd_exe")/ctr" --address "$socket" --namespace k8s.io)
fi

"${ctr[@]}" images import "$remote_archive"
match_ref="$expected_image"
match="$("${ctr[@]}" images ls | awk -v expected="$match_ref" '$1 == expected {print; found=1} END {if (!found) exit 1}')"
if [ -z "$match" ]; then
  match_ref="$canonical_image"
  match="$("${ctr[@]}" images ls | awk -v expected="$match_ref" '$1 == expected {print; found=1} END {if (!found) exit 1}')"
fi
[ -n "$match" ] || exit 1
if [ "$match_ref" != "$expected_image" ]; then
  "${ctr[@]}" images tag "$match_ref" "$expected_image"
fi
[ "$canonical_image" = "$expected_image" ] || "${ctr[@]}" images tag "$expected_image" "$canonical_image"
[ -z "$platform" ] || printf '%s\n' "$match" | grep -Fq "$platform"
printf '%s\n' "$match"
rm -f "$remote_archive"
REMOTE
}

declare -a pids=()
declare -a labels=()
for target in "${targets[@]}"; do
  import_target "$target" >"/tmp/kubecell-distribute-${target//[^a-zA-Z0-9]/_}.log" 2>&1 &
  pids+=("$!")
  labels+=("$target")
done

status=0
for index in "${!pids[@]}"; do
  if ! wait "${pids[$index]}"; then
    echo "[${labels[$index]}] import failed" >&2
    status=1
  fi
  log_file="/tmp/kubecell-distribute-${labels[$index]//[^a-zA-Z0-9]/_}.log"
  sed "s/^/[${labels[$index]}] /" "$log_file"
  rm -f "$log_file"
done
exit "$status"
