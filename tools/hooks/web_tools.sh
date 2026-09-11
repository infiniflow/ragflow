#!/bin/sh
#
# Resolve the oxlint/oxfmt pair used by the web pre-commit jobs, installing the
# versions pinned by web/package-lock.json on first use.
#
# Prints the directory holding both binaries on stdout; every progress message
# goes to stderr, so callers can use `BIN=$(tools/hooks/web_tools.sh)`.
#
# POSIX sh: lefthook runs `run:` blocks through /bin/sh, which is dash on most
# Linux runners.
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
web_dir="$repo_root/web"
lock="$web_dir/package-lock.json"

if ! command -v node >/dev/null 2>&1 || ! command -v npm >/dev/null 2>&1; then
  echo "web_tools: node/npm not found — they are required to lint and format web/ files" >&2
  exit 1
fi

# Version package-lock.json pins $1 to.
locked_version() {
  node -p "require('$lock').packages['node_modules/$1'].version"
}

# Version of an installed package, empty when it is not there.
installed_version() {
  node -p "try{require('$1/package.json').version}catch(e){''}"
}

oxlint_version=$(locked_version oxlint)
oxfmt_version=$(locked_version oxfmt)

# A contributor who ran `npm install` in web/ already has the pinned pair;
# reuse it instead of keeping a second copy in sync.
if [ "$(installed_version "$web_dir/node_modules/oxlint")" = "$oxlint_version" ] &&
  [ "$(installed_version "$web_dir/node_modules/oxfmt")" = "$oxfmt_version" ]; then
  echo "$web_dir/node_modules/.bin"
  exit 0
fi

# Otherwise keep the binaries in a version-keyed cache OUTSIDE the repo. Inside
# web/node_modules they would be wiped by the next `npm ci`, and npm would treat
# the directory as a stray package.
cache="${XDG_CACHE_HOME:-$HOME/.cache}/ragflow/web-tools/oxlint-$oxlint_version-oxfmt-$oxfmt_version"
cache_bin="$cache/node_modules/.bin"

if [ ! -x "$cache_bin/oxlint" ] || [ ! -x "$cache_bin/oxfmt" ]; then
  echo "web_tools: installing oxlint@$oxlint_version oxfmt@$oxfmt_version" >&2
  mkdir -p "$cache"
  printf '{"name":"ragflow-web-tools","version":"1.0.0","private":true}\n' >"$cache/package.json"
  # Read the registry from web/.npmrc explicitly: with --prefix pointing at the
  # cache, npm's own project-config lookup no longer lands on web/.npmrc.
  registry=$(cd "$web_dir" && npm config get registry)
  # oxlint and oxfmt are prebuilt Rust binaries (5 packages, ~35 MB), so this
  # needs none of web/'s ~1.5 GB dependency tree.
  npm install --prefix "$cache" --registry "$registry" \
    --no-audit --no-fund --loglevel=error \
    "oxlint@$oxlint_version" "oxfmt@$oxfmt_version" >&2
fi

echo "$cache_bin"
