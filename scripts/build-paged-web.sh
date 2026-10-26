#!/usr/bin/env bash
# Build the pinned frontend plus our small source patch; never download a release dist.
set -euo pipefail

ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
readonly ROOT
readonly SOURCE_URL=https://github.com/AlistGo/alist-web.git
readonly SOURCE_REV=907756a8d5dff27d6b73036734de0a6533ace553
readonly ROUTER_REV=f77b206e8592bb56d2d001e9488de5a54a13699b
readonly PATCH="$ROOT/web-patches/115-on-demand.patch"

# This build must not run accidentally on a small production server.
if [[ ${CI:-} != true && ${ALIST_ALLOW_WEB_BUILD:-} != 1 ]]; then
  echo 'Frontend builds are disabled locally. Use the manual 115-on-demand GitHub workflow.' >&2
  exit 1
fi
if [[ -r /proc/meminfo ]]; then
  available_kib=$(awk '/^MemAvailable:/ {print $2}' /proc/meminfo)
  if [[ ! $available_kib =~ ^[0-9]+$ ]] || (( available_kib < 3145728 )); then
    echo 'Refusing frontend build: at least 3 GiB available RAM is required (swap does not count).' >&2
    exit 1
  fi
fi
for memory_limit in /sys/fs/cgroup/memory.max /sys/fs/cgroup/memory/memory.limit_in_bytes; do
  if [[ -r "$memory_limit" ]]; then
    read -r limit < "$memory_limit"
    if [[ $limit =~ ^[0-9]+$ ]] && (( limit < 4294967296 )); then
      echo 'Refusing frontend build: container memory limit is below 4 GiB.' >&2
      exit 1
    fi
  fi
done

# The development image keeps Node here; ordinary installations use PATH.
if ! command -v node >/dev/null 2>&1 && [[ -x "$HOME/.local/node/bin/node" ]]; then
  export PATH="$HOME/.local/node/bin:$PATH"
fi
command -v node >/dev/null || { echo 'Node.js is required' >&2; exit 1; }
command -v git >/dev/null || { echo 'git is required' >&2; exit 1; }
[[ -f "$PATCH" ]] || { echo "Missing patch: $PATCH" >&2; exit 1; }
if command -v corepack >/dev/null 2>&1; then
  PNPM=(corepack pnpm@9.9.0)
elif command -v pnpm >/dev/null 2>&1 && [[ $(pnpm --version) == 9.9.0 ]]; then
  PNPM=(pnpm)
else
  echo 'Corepack or pnpm 9.9.0 is required' >&2
  exit 1
fi
export NODE_OPTIONS="${NODE_OPTIONS:---max-old-space-size=2048}"
export HUSKY=0
WORK=$(mktemp -d)
STAGE=
cleanup() {
  rm -rf -- "$WORK"
  if [[ -n "$STAGE" ]]; then rm -rf -- "$STAGE"; fi
}
trap cleanup EXIT

git init -q "$WORK/web"
git -C "$WORK/web" remote add origin "$SOURCE_URL"
git -C "$WORK/web" fetch --depth=1 origin "$SOURCE_REV"
git -C "$WORK/web" checkout --detach FETCH_HEAD
[[ $(git -C "$WORK/web" rev-parse HEAD) == "$SOURCE_REV" ]]
git -C "$WORK/web" submodule update --init --recursive --depth=1
[[ $(git -C "$WORK/web/solid-router" rev-parse HEAD) == "$ROUTER_REV" ]]
git -C "$WORK/web" apply --check "$PATCH"
git -C "$WORK/web" apply "$PATCH"
(
  cd -- "$WORK/web"
  "${PNPM[@]}" install --frozen-lockfile
  node --test scripts/paging.test.mjs
  "${PNPM[@]}" build
)
[[ -s "$WORK/web/dist/index.html" ]] || { echo 'Build produced no index.html' >&2; exit 1; }
[[ -d "$ROOT/public" && ! -L "$ROOT/public" && ! -L "$ROOT/public/dist" ]] || {
  echo 'Refusing an unsafe public/dist path (symlink or missing public directory)' >&2
  exit 1
}
# Stage on the same filesystem. Preserve the tracked placeholder documentation.
STAGE=$(mktemp -d "$ROOT/public/.paged-web.XXXXXXXX")
cp -a -- "$WORK/web/dist/." "$STAGE/"
if [[ -f "$ROOT/public/dist/README.md" ]]; then
  cp -- "$ROOT/public/dist/README.md" "$STAGE/README.md"
fi
rm -rf -- "$ROOT/public/dist"
mv -- "$STAGE" "$ROOT/public/dist"
STAGE=
printf 'Installed patched frontend %s into %s/public/dist\n' "$SOURCE_REV" "$ROOT"
