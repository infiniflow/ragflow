#!/usr/bin/env bash
#
# Builds the Windows-only native artifacts the desktop bundle links and ships:
#
#   <cache>/office_oxide/v<ver>/lib/liboffice_oxide.dll.a    MinGW import library
#   <cache>/pdfium-static/v<ver>/pdfium.dll                  the DLL itself
#   <cache>/pdfium-static/v<ver>/lib/libpdfium.dll.a         MinGW import library
#   <cache>/onnxruntime/windows-x64-v<ver>/onnxruntime.dll   the DLL itself
#   <cache>/pcre2/v<ver>/lib/libpcre2-8.a                    static, MinGW-built
#
# <cache> is ragflow_native_libs_dir() (~/ragflow-native-libs by default). These
# paths are exactly what build.sh probes, so running this script is the whole
# Windows provisioning step.
#
# Why a separate step: cgo on Windows links with MinGW-w64, and MinGW's ld
# cannot consume the archives the upstream Windows releases publish — they are
# MSVC objects, so they reference the MSVC C++ ABI, the static CRT's
# __security_cookie/__chkstk and the STL's __std_* helpers. A DLL with a
# generated import library is the only shape both sides understand: GNU ld
# links the .dll.a, the loader finds the DLL beside the executable.
#
#   office_oxide    the release already ships office_oxide.dll, so the import
#                   library is generated from it (gendef + dlltool): no MSVC.
#   pdfium          the release ships lib/pdfium.lib only, so it has to be
#                   relinked into pdfium.dll first: that needs MSVC link.exe.
#   onnxruntime     same, plus tools/scripts/windows/std_unique_shim.c, which
#                   supplies the one STL helper the archive's newer toolset
#                   expects and the relink cannot find.
#   pcre2           the tokenizer's regex engine, which Windows does not ship at
#                   all: the pinned release is compiled here with the same
#                   MinGW toolchain that links it (cmake + ninja or mingw32-make).
#
# Inputs are downloaded from the pinned releases when they are missing (same
# versions build.sh and tools/scripts/download_dev_deps.py resolve), so this is
# the only Windows provisioning step:
#
#   bash tools/scripts/build_windows_native_dlls.sh              # fetch + build
#   bash tools/scripts/build_windows_native_dlls.sh --check      # report only
#   bash tools/scripts/build_windows_native_dlls.sh --no-download
#   bash tools/scripts/build_windows_native_dlls.sh --force      # rebuild
#
# Escapes: RAGFLOW_ORT_DLL / RAGFLOW_PDFIUM_DLL point at an existing DLL instead
# of building one (build.sh honours RAGFLOW_ORT_DLL the same way), GH_PROXY
# prefixes the download URLs for a mirror, and RAGFLOW_MSVC_VCVARS64 names
# vcvars64.bat when Visual Studio is installed somewhere vswhere cannot see.
#
# Linux and macOS need none of this: they link the static archives from the
# release bundles directly, so this script is a no-op there.

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$here/../.." && pwd)"

usage() {
  cat <<'EOF'
Builds the Windows native DLLs, MinGW import libraries and the static pcre2 the
desktop bundle needs, downloading the pinned release archives when they are
missing.

  bash tools/scripts/build_windows_native_dlls.sh              # fetch + build
  bash tools/scripts/build_windows_native_dlls.sh --check      # report only
  bash tools/scripts/build_windows_native_dlls.sh --no-download
  bash tools/scripts/build_windows_native_dlls.sh --force      # rebuild

Needs the MinGW-w64 toolchain, cmake (plus ninja or mingw32-make), and MSVC's
link.exe for pdfium/onnxruntime — the only part of the Windows build that does.

Overrides: RAGFLOW_ORT_DLL, RAGFLOW_PDFIUM_DLL (use an existing DLL instead of
building one), GH_PROXY (mirror prefix for the downloads), and
RAGFLOW_MSVC_VCVARS64 (vcvars64.bat when vswhere cannot find it).

Linux and macOS link the static archives directly and are unaffected.
EOF
}

mode=build
download=on
force=off
for arg in "$@"; do
  case "$arg" in
    --check) mode=check ;;
    --no-download) download=off ;;
    --force) force=on ;;
    --help|-h) usage; exit 0 ;;
    *) echo "build_windows_native_dlls: unknown argument: $arg" >&2; usage >&2; exit 2 ;;
  esac
done

case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) ;;
  *)
    echo "build_windows_native_dlls: $(uname -s) links the static archives directly; nothing to do."
    exit 0
    ;;
esac

say() { printf '%s\n' "$*"; }
step() { printf '\n=== %s ===\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

# ── versions: read from the sources that already own them ──────────────────
# No copy of any version lives here; a bump must not need to touch this file.
source "$repo_root/tools/scripts/native_lib_version.sh"
NATIVE_LIBS_DIR="$(ragflow_native_libs_dir)"

PDFIUM_VERSION="$(pdfium_static_version "$repo_root")"
[ -n "$PDFIUM_VERSION" ] || die "pdfium_static_version produced nothing"
PCRE2_VERSION="$(pcre2_static_version)"
[ -n "$PCRE2_VERSION" ] || die "pcre2_static_version produced nothing"
OFFICE_OXIDE_VERSION="$(go_module_version github.com/yfedoseev/office_oxide/go "$repo_root")"
[ -n "$OFFICE_OXIDE_VERSION" ] || die "office_oxide version not found in go.mod"
PDF_OXIDE_VERSION="$(go_module_version github.com/yfedoseev/pdf_oxide/go "$repo_root")"
[ -n "$PDF_OXIDE_VERSION" ] || die "pdf_oxide version not found in go.mod"
ORT_VERSION="$(sed -n 's/^const DeepDocORTVersion = "\([^"]*\)".*/\1/p' "$repo_root/internal/common/environments.go")"
[ -n "$ORT_VERSION" ] || die "DeepDocORTVersion not found in internal/common/environments.go"
# The release tag carries the patch suffix (the Windows archive is the same
# minimal build as the other platforms); download_dev_deps.py owns that string.
ORT_TAG="$(sed -n 's/^ORT_RELEASE_TAG = "\([^"]*\)".*/\1/p' "$repo_root/tools/scripts/download_dev_deps.py")"
[ -n "$ORT_TAG" ] || die "ORT_RELEASE_TAG not found in tools/scripts/download_dev_deps.py"
ORT_SUFFIX="${ORT_TAG#onnxruntime-v${ORT_VERSION}}"

OFFICE_OXIDE_DIR="$NATIVE_LIBS_DIR/office_oxide/v$OFFICE_OXIDE_VERSION"
PDFIUM_DIR="$NATIVE_LIBS_DIR/pdfium-static/v$PDFIUM_VERSION"
PDF_OXIDE_DIR="$NATIVE_LIBS_DIR/pdf_oxide/v$PDF_OXIDE_VERSION"
ORT_DIR="$NATIVE_LIBS_DIR/onnxruntime/windows-x64-v$ORT_VERSION$ORT_SUFFIX"
# The C++ build takes pcre2 from the same version-keyed cache
# (PCRE2_STATIC_PREFIX in build.sh) — Windows has no system pcre2.
PCRE2_DIR="$NATIVE_LIBS_DIR/pcre2/v$PCRE2_VERSION"
PCRE2_SRC="$NATIVE_LIBS_DIR/_build/pcre2-$PCRE2_VERSION"
DOWNLOADS="$NATIVE_LIBS_DIR/_downloads"

say "cache: $NATIVE_LIBS_DIR"
say "  office_oxide v$OFFICE_OXIDE_VERSION"
say "  pdfium       v$PDFIUM_VERSION"
say "  pdf_oxide    v$PDF_OXIDE_VERSION"
say "  onnxruntime  v$ORT_VERSION ($ORT_TAG)"
say "  pcre2        v$PCRE2_VERSION"

# ── inputs ─────────────────────────────────────────────────────────────────
fetch() { # url dest
  local url="${GH_PROXY:-}$1" dest="$2"
  [ -s "$dest" ] && { say "  cached  $(basename "$dest")"; return 0; }
  mkdir -p "$(dirname "$dest")"
  say "  fetch   $url"
  curl -fL --retry 2 --connect-timeout 20 --max-time 1800 -o "$dest.part" "$url" ||
    { rm -f "$dest.part"; die "download failed: $url"; }
  mv "$dest.part" "$dest"
}

unpack_zip() { # archive dest
  mkdir -p "$2"
  if command -v unzip >/dev/null 2>&1; then
    unzip -oq "$1" -d "$2"
  else
    powershell -NoProfile -Command \
      "Expand-Archive -LiteralPath '$(cygpath -w "$1")' -DestinationPath '$(cygpath -w "$2")' -Force" >/dev/null
  fi
}

unpack_tgz() { # archive dest
  mkdir -p "$2"
  tar -xf "$1" -C "$2"
}

ensure_office_oxide() {
  [ -f "$OFFICE_OXIDE_DIR/lib/office_oxide.dll" ] && [ -f "$OFFICE_OXIDE_DIR/include/office_oxide_c/office_oxide.h" ] && return 0
  step "office_oxide v$OFFICE_OXIDE_VERSION"
  [ "$download" = on ] || die "$OFFICE_OXIDE_DIR is incomplete and --no-download was given"
  local zip="$DOWNLOADS/office_oxide-v$OFFICE_OXIDE_VERSION-win-x64.zip"
  fetch "https://github.com/yfedoseev/office_oxide/releases/download/v$OFFICE_OXIDE_VERSION/native-windows-x86_64.zip" "$zip"
  rm -rf "$OFFICE_OXIDE_DIR"
  unpack_zip "$zip" "$OFFICE_OXIDE_DIR"
}

ensure_pdfium() {
  [ -f "$PDFIUM_DIR/lib/pdfium.lib" ] && [ -f "$PDFIUM_DIR/include/fpdfview.h" ] && return 0
  step "pdfium v$PDFIUM_VERSION"
  [ "$download" = on ] || die "$PDFIUM_DIR is incomplete and --no-download was given"
  local tgz="$DOWNLOADS/pdfium-v$PDFIUM_VERSION-win-x64.tgz"
  fetch "https://github.com/kognitos/pdfium-static/releases/download/chromium%2F$PDFIUM_VERSION/pdfium-win-x64-static.tgz" "$tgz"
  rm -rf "$PDFIUM_DIR"
  unpack_tgz "$tgz" "$PDFIUM_DIR"
}

ensure_pdf_oxide() {
  [ -f "$PDF_OXIDE_DIR/lib/windows_amd64/libpdf_oxide.a" ] && return 0
  step "pdf_oxide v$PDF_OXIDE_VERSION"
  [ "$download" = on ] || die "$PDF_OXIDE_DIR is incomplete and --no-download was given"
  local tgz="$DOWNLOADS/pdf_oxide-v$PDF_OXIDE_VERSION-win-amd64.tar.gz"
  fetch "https://github.com/yfedoseev/pdf_oxide/releases/download/v$PDF_OXIDE_VERSION/pdf_oxide-go-ffi-windows-amd64.tar.gz" "$tgz"
  rm -rf "$PDF_OXIDE_DIR"
  unpack_tgz "$tgz" "$PDF_OXIDE_DIR"
}

ensure_onnxruntime() {
  [ -f "$ORT_DIR/lib/onnxruntime.lib" ] && [ -f "$ORT_DIR/include/onnxruntime_c_api.h" ] && return 0
  step "onnxruntime v$ORT_VERSION"
  [ "$download" = on ] || die "$ORT_DIR is incomplete and --no-download was given"
  local zip="$DOWNLOADS/onnxruntime-v$ORT_VERSION-win-x64.zip"
  fetch "https://github.com/infiniflow/ragflow-build/releases/download/$ORT_TAG/onnxruntime-v$ORT_VERSION-windows-x64.zip" "$zip"
  local tmp="$NATIVE_LIBS_DIR/onnxruntime/.unpack-$$"
  rm -rf "$tmp"
  unpack_zip "$zip" "$tmp"
  # The archive's top directory is named after the release target, not the
  # cache layout build.sh probes (onnxruntime/*/onnxruntime.dll).
  rm -rf "$ORT_DIR"
  mkdir -p "$(dirname "$ORT_DIR")"
  mv "$tmp/onnxruntime-v$ORT_VERSION-windows-x64" "$ORT_DIR" 2>/dev/null ||
    mv "$tmp"/* "$ORT_DIR" 2>/dev/null || true
  rm -rf "$tmp"
  [ -d "$ORT_DIR" ] || die "unexpected layout inside $zip"
}

# pcre2 is not a prebuilt download on Windows: there is no system copy, so the
# C++ tokenizer links an archive built from the pinned release with the MinGW
# toolchain. The source tree is kept out of the cache layout and rebuilt on
# demand (a minute), because the archive itself is the only artifact build.sh
# looks for (PCRE2_STATIC_PREFIX/lib/libpcre2-8.a).
ensure_pcre2() {
  [ -f "$PCRE2_DIR/lib/libpcre2-8.a" ] && return 0
  [ -d "$PCRE2_SRC" ] && return 0
  step "pcre2 v$PCRE2_VERSION (source)"
  [ "$download" = on ] || die "$PCRE2_SRC is missing and --no-download was given"
  local tgz="$DOWNLOADS/pcre2-$PCRE2_VERSION.tar.gz"
  fetch "https://github.com/PCRE2Project/pcre2/releases/download/pcre2-$PCRE2_VERSION/pcre2-$PCRE2_VERSION.tar.gz" "$tgz"
  rm -rf "$PCRE2_SRC"
  mkdir -p "$NATIVE_LIBS_DIR/_build"
  unpack_tgz "$tgz" "$NATIVE_LIBS_DIR/_build"
  [ -d "$PCRE2_SRC" ] || die "unexpected layout inside $tgz"
}

# ── toolchains ─────────────────────────────────────────────────────────────
need_mingw_tool() {
  command -v "$1" >/dev/null 2>&1 ||
    die "$1 not found; it ships with the MinGW-w64 toolchain cgo builds with (add its bin/ to PATH)"
}

VCVARS64="${RAGFLOW_MSVC_VCVARS64:-}"

find_msvc() {
  [ -n "$VCVARS64" ] && [ -f "$VCVARS64" ] && return 0
  local pf86 vswhere vs
  pf86="$(printenv 'ProgramFiles(x86)' || true)"
  [ -n "$pf86" ] || return 1
  vswhere="$pf86/Microsoft Visual Studio/Installer/vswhere.exe"
  [ -f "$vswhere" ] || return 1
  vs="$("$vswhere" -latest -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath 2>/dev/null | tr -d '\r')"
  [ -n "$vs" ] || return 1
  VCVARS64="$vs/VC/Auxiliary/Build/vcvars64.bat"
  [ -f "$VCVARS64" ] || return 1
  say "  msvc    $vs"
}

require_msvc() {
  find_msvc && return 0
  cat >&2 <<'EOF'

The Windows pdfium and onnxruntime releases ship static MSVC archives only, so
turning them into loadable DLLs needs MSVC's linker. Install the toolchain once:

    winget install --id Microsoft.VisualStudio.2022.BuildTools \
        --override "--add Microsoft.VisualStudio.Component.VC.Tools.x86.x64 --passive"

Then rerun this script. If Visual Studio lives outside vswhere's reach, point
RAGFLOW_MSVC_VCVARS64 at VC\Auxiliary\Build\vcvars64.bat.

Alternatively drop ready-made DLLs in place and rerun with them referenced:
    RAGFLOW_PDFIUM_DLL=<path> RAGFLOW_ORT_DLL=<path> bash $0
EOF
  exit 1
}

# Runs a batch file under a shell that already has the MSVC environment, which
# is the only supported way to get link.exe's LIB/INCLUDE paths right.
run_msvc_batch() { # batch-file
  cmd //c "$(cygpath -w "$1")"
}

# cmd.exe tolerates LF-only batch files in practice, but not reliably across
# versions, so CRLF is written explicitly instead of inheriting the shell's
# line endings.
msvc_batch() { # out-file, then commands on stdin
  local bat="$1"
  {
    printf '@echo off\n'
    printf 'call "%s" >nul || exit /b 1\n' "$(cygpath -w "$VCVARS64")"
    cat
  } | sed 's/$/\r/' > "$bat"
}

# ── static library: pcre2 ──────────────────────────────────────────────────
# build.sh's C++ step links PCRE2_STATIC_PREFIX/lib/libpcre2-8.a and there is no
# system pcre2 on Windows, so the archive is built here from the pinned release
# with the same MinGW toolchain that will link it. Only the 8-bit library is
# needed (the analyzer uses PCRE2_UTF/PCRE2_UCP on 8-bit patterns); the 16/32-bit
# variants, tests and pcre2grep would triple the build for nothing.
build_pcre2() {
  local archive="$PCRE2_DIR/lib/libpcre2-8.a"
  if [ "$mode" = build ] && [ "$force" = off ] && [ -f "$archive" ]; then
    say "  libpcre2-8.a: up to date"
    return 0
  fi
  if [ "$mode" = check ]; then
    [ -f "$archive" ] || say "  libpcre2-8.a: MISSING (run without --check)"
    return 0
  fi
  step "libpcre2-8.a (pcre2 v$PCRE2_VERSION)"
  need_mingw_tool gcc
  command -v cmake >/dev/null 2>&1 ||
    die "cmake not found; it is required to build pcre2 from source"
  [ -d "$PCRE2_SRC" ] || die "$PCRE2_SRC is missing; rerun without --no-download"

  # Ninja when it is available, otherwise the MinGW makefile generator: the
  # winlibs toolchain ships mingw32-make but not make, so the Unix Makefiles
  # generator would look for a program that is not there.
  local -a generator
  if command -v ninja >/dev/null 2>&1; then
    generator=(-G Ninja)
  elif command -v mingw32-make >/dev/null 2>&1; then
    generator=(-G "MinGW Makefiles")
  else
    die "neither ninja nor mingw32-make is available to build pcre2"
  fi

  local build="$NATIVE_LIBS_DIR/_build/pcre2-$PCRE2_VERSION-mingw"
  rm -rf "$build"
  cmake -S "$PCRE2_SRC" -B "$build" "${generator[@]}" \
    -DCMAKE_BUILD_TYPE=Release \
    -DBUILD_SHARED_LIBS=OFF \
    -DPCRE2_BUILD_PCRE2_8=ON \
    -DPCRE2_BUILD_PCRE2_16=OFF \
    -DPCRE2_BUILD_PCRE2_32=OFF \
    -DPCRE2_BUILD_TESTS=OFF \
    -DPCRE2_BUILD_PCRE2GREP=OFF >/dev/null
  cmake --build "$build" --config Release --parallel >/dev/null
  [ -f "$build/libpcre2-8.a" ] || die "cmake produced no libpcre2-8.a in $build"
  mkdir -p "$PCRE2_DIR/lib"
  cp -f "$build/libpcre2-8.a" "$PCRE2_DIR/lib/"
  rm -rf "$build"
  say "  wrote $archive"
}

# ── import libraries and DLLs ──────────────────────────────────────────────
# office_oxide: the DLL is part of the release, so the import library is the
# only thing to derive. gendef reads the PE export table; dlltool writes the
# GNU archive with the DLL name recorded (-D), which is what the loader looks up.
build_office_oxide() {
  local lib_archive="$OFFICE_OXIDE_DIR/lib/liboffice_oxide.dll.a"
  local dll="$OFFICE_OXIDE_DIR/lib/office_oxide.dll"
  if [ "$mode" = build ] && [ "$force" = off ] && [ -f "$lib_archive" ] && [ ! "$dll" -nt "$lib_archive" ]; then
    say "  office_oxide.dll.a: up to date"
    return 0
  fi
  if [ "$mode" = check ]; then
    [ -f "$lib_archive" ] || say "  office_oxide.dll.a: MISSING (run without --check)"
    return 0
  fi
  step "liboffice_oxide.dll.a"
  need_mingw_tool gendef
  need_mingw_tool dlltool
  ( cd "$OFFICE_OXIDE_DIR/lib" &&
    [ -f office_oxide.def ] || gendef office_oxide.dll >/dev/null &&
    dlltool -d office_oxide.def -D office_oxide.dll -l liboffice_oxide.dll.a )
  say "  wrote $lib_archive"
}

# pdfium: the release ships lib/pdfium.lib (MSVC static). Relink it into a DLL
# exporting every function the archive defines, then generate the import
# library from that same list so DLL and .dll.a can never disagree.
build_pdfium() {
  local dll="$PDFIUM_DIR/pdfium.dll"
  local lib_archive="$PDFIUM_DIR/lib/libpdfium.dll.a"
  local exports="$PDFIUM_DIR/exports.txt"
  local def="$PDFIUM_DIR/pdfium.def"

  if [ -n "${RAGFLOW_PDFIUM_DLL:-}" ]; then
    step "pdfium.dll (external)"
    [ -f "$RAGFLOW_PDFIUM_DLL" ] || die "RAGFLOW_PDFIUM_DLL is not a file: $RAGFLOW_PDFIUM_DLL"
    install -m 0644 "$RAGFLOW_PDFIUM_DLL" "$dll"
  fi

  if [ "$mode" = build ] && [ "$force" = off ] && [ -f "$dll" ] && [ -f "$lib_archive" ] &&
     [ ! "$PDFIUM_DIR/lib/pdfium.lib" -nt "$dll" ] && [ ! "$dll" -nt "$lib_archive" ]; then
    say "  pdfium.dll + libpdfium.dll.a: up to date"
    return 0
  fi

  if [ "$mode" = check ]; then
    [ -f "$dll" ] || say "  pdfium.dll: MISSING (run without --check)"
    [ -f "$lib_archive" ] || say "  libpdfium.dll.a: MISSING (run without --check)"
    return 0
  fi

  need_mingw_tool nm
  need_mingw_tool dlltool

  if [ ! -f "$dll" ] || [ "$force" = on ]; then
    step "pdfium.dll"
    require_msvc
    say "  reading the archive's function symbols (this takes a minute)"
    nm --defined-only -g "$PDFIUM_DIR/lib/pdfium.lib" |
      awk '$2 ~ /^[TtWw]$/ {print $3}' |
      grep -E '^[A-Za-z]' | sort -u > "$exports"
    [ -s "$exports" ] || die "no exported functions found in lib/pdfium.lib"
    { printf 'LIBRARY pdfium\nEXPORTS\n'; sed 's/^/    /' "$exports"; } > "$def"

    # A failed relink must not cost the cache a working DLL, so the previous
    # one stays until link.exe has actually written the new one. The import
    # library link.exe would emit is named out of the way: the one the Go link
    # consumes is generated below from the matching export list.
    [ -f "$dll" ] && cp -f "$dll" "$dll.bak"
    local bat="$PDFIUM_DIR/.link.bat"
    msvc_batch "$bat" <<EOF
cd /d "$(cygpath -w "$PDFIUM_DIR")" || exit /b 1
link /nologo /DLL /MACHINE:X64 /DEF:pdfium.def /IMPLIB:pdfium.implib /OUT:pdfium.dll lib\\pdfium.lib user32.lib gdi32.lib advapi32.lib || exit /b 1
EOF
    run_msvc_batch "$bat"
    rm -f "$bat"
    if [ ! -f "$dll" ]; then
      [ -f "$dll.bak" ] && mv -f "$dll.bak" "$dll"
      die "link.exe produced no pdfium.dll"
    fi
    rm -f "$dll.bak" "$PDFIUM_DIR"/*.implib "$PDFIUM_DIR"/*.exp
    say "  wrote $dll"
  fi

  step "libpdfium.dll.a"
  [ -f "$def" ] || die "$def is missing; rerun with --force to relink the DLL"
  dlltool -d "$def" -D pdfium.dll -l "$lib_archive"
  say "  wrote $lib_archive"
}

# onnxruntime: same relink, but the export surface is the single C entry point
# the Go binding resolves at run time — the API table hangs off it.
build_onnxruntime() {
  local dll="$ORT_DIR/onnxruntime.dll"
  local def="$ORT_DIR/exports.def"

  if [ -n "${RAGFLOW_ORT_DLL:-}" ]; then
    step "onnxruntime.dll (external)"
    [ -f "$RAGFLOW_ORT_DLL" ] || die "RAGFLOW_ORT_DLL is not a file: $RAGFLOW_ORT_DLL"
    install -m 0644 "$RAGFLOW_ORT_DLL" "$dll"
  fi

  if [ "$mode" = build ] && [ "$force" = off ] && [ -f "$dll" ] && [ ! "$ORT_DIR/lib/onnxruntime.lib" -nt "$dll" ]; then
    say "  onnxruntime.dll: up to date"
    return 0
  fi
  [ "$mode" = check ] && { [ -f "$dll" ] || say "  onnxruntime.dll: MISSING (run without --check)"; return 0; }

  step "onnxruntime.dll"
  require_msvc
  printf 'LIBRARY onnxruntime\nEXPORTS\n    OrtGetApiBase\n' > "$def"

  # dbghelp/shell32/advapi32 are what the archive's telemetry and diagnostic
  # paths import; without them the relink stops on CommandLineToArgvW.
  local log="$ORT_DIR/.link.log"
  local bat="$ORT_DIR/.link.bat"
  link_ort() { # extra-objects (Windows path or empty)
    msvc_batch "$bat" <<EOF
cd /d "$(cygpath -w "$ORT_DIR")" || exit /b 1
link /nologo /DLL /MACHINE:X64 /DEF:exports.def /IMPLIB:onnxruntime.implib /OUT:onnxruntime.dll lib\\onnxruntime.lib shell32.lib advapi32.lib dbghelp.lib $1 > .link.log 2>&1
EOF
    run_msvc_batch "$bat" >/dev/null 2>&1
    local rc=$?
    rm -f "$bat"
    return $rc
  }

  [ -f "$dll" ] && cp -f "$dll" "$dll.bak"
  if ! link_ort ""; then
    # The archive was built against a newer MSVC STL than some toolchains
    # provide: there, `std::unique` over 8-byte elements calls __std_unique_8,
    # which lives in the STL runtime and is therefore missing from the archive.
    # The shim supplies it. Toolchains whose STL already exports it must not
    # link the shim (LNK2005), so it is only added for that one failure.
    if grep -qi '__std_unique_8' "$log"; then
      say "  adding the __std_unique_8 shim"
      msvc_batch "$bat" <<EOF
cd /d "$(cygpath -w "$ORT_DIR")" || exit /b 1
cl /nologo /c /MD /TC /Fo:std_unique_shim.obj "$(cygpath -w "$here/windows/std_unique_shim.c")" || exit /b 1
EOF
      run_msvc_batch "$bat" || die "could not compile std_unique_shim.c"
      rm -f "$bat"
      link_ort "std_unique_shim.obj" || true
    fi
  fi
  rm -f "$ORT_DIR"/*.obj

  if [ ! -f "$dll" ]; then
    [ -f "$dll.bak" ] && mv -f "$dll.bak" "$dll"
    [ -f "$log" ] && { printf '\nlink.exe said:\n' >&2; cat "$log" >&2; }
    die "link.exe produced no onnxruntime.dll"
  fi
  rm -f "$dll.bak" "$log" "$ORT_DIR"/*.implib "$ORT_DIR"/*.exp
  say "  wrote $dll"
}

# ── verification ───────────────────────────────────────────────────────────
verify() {
  step "verify"
  local ok=0
  local archive="$OFFICE_OXIDE_DIR/lib/liboffice_oxide.dll.a"
  if [ -f "$archive" ] && nm "$archive" 2>/dev/null | grep -q '__imp_office_'; then
    say "  office_oxide: import library exposes the DLL's entry points"
  elif [ -f "$archive" ]; then
    say "  office_oxide: import library present"
  else
    warn "office_oxide: no liboffice_oxide.dll.a"; ok=1
  fi
  if [ -f "$PDFIUM_DIR/pdfium.dll" ]; then
    say "  pdfium: pdfium.dll $(du -h "$PDFIUM_DIR/pdfium.dll" | cut -f1)"
  else
    warn "pdfium: no pdfium.dll"; ok=1
  fi
  if [ -f "$PDFIUM_DIR/lib/libpdfium.dll.a" ] && nm "$PDFIUM_DIR/lib/libpdfium.dll.a" 2>/dev/null | grep -q '__imp_FPDF_InitLibrary'; then
    say "  pdfium: import library exposes FPDF_InitLibrary"
  else
    warn "pdfium: libpdfium.dll.a missing or does not import FPDF_InitLibrary"; ok=1
  fi
  if [ -f "$ORT_DIR/onnxruntime.dll" ]; then
    say "  onnxruntime: onnxruntime.dll $(du -h "$ORT_DIR/onnxruntime.dll" | cut -f1)"
  else
    warn "onnxruntime: no onnxruntime.dll (DeepDoc cannot start without it)"; ok=1
  fi
  if [ -f "$PDF_OXIDE_DIR/lib/windows_amd64/libpdf_oxide.a" ]; then
    say "  pdf_oxide: libpdf_oxide.a present"
  else
    warn "pdf_oxide: libpdf_oxide.a missing"; ok=1
  fi
  if [ -f "$PCRE2_DIR/lib/libpcre2-8.a" ] && nm "$PCRE2_DIR/lib/libpcre2-8.a" 2>/dev/null | grep -q ' T _*pcre2_compile_8$'; then
    say "  pcre2: libpcre2-8.a exposes pcre2_compile_8"
  elif [ -f "$PCRE2_DIR/lib/libpcre2-8.a" ]; then
    warn "pcre2: libpcre2-8.a present but pcre2_compile_8 was not found"; ok=1
  else
    warn "pcre2: no libpcre2-8.a (the C++ tokenizer cannot build without it)"; ok=1
  fi
  return "$ok"
}

if [ "$mode" = check ]; then
  build_pcre2
  build_office_oxide
  build_pdfium
  build_onnxruntime
  verify
  exit $?
fi

ensure_office_oxide
ensure_pdfium
ensure_pdf_oxide
ensure_onnxruntime
ensure_pcre2

build_pcre2
build_office_oxide
build_pdfium
build_onnxruntime
verify

cat <<EOF

Native libraries are in place; bin/ gets the DLLs from build.sh, and the app
bundles them via desktop/src-tauri/tauri.windows.conf.json. The C++ tokenizer
takes libpcre2-8.a from the same cache. Next:

    bash desktop/build.sh
EOF
