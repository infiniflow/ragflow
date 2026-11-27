#!/usr/bin/env bash
# Run: bash test/build_cpp_osx_architectures.sh
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../build.sh
source "${SCRIPT_DIR}/../build.sh"

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
failures=0

assert_cmake_args() {
    local description="$1" os="$2" arch="$3" mode="$4" extra_args="$5"
    shift 5
    local actual expected
    local args_file="$work_dir/cmake-args"
    rm -f "$args_file"

    # Run the actual build dispatch in a subshell. Stop at the CMake boundary
    # before compiling so the macOS cases can run on any build host.
    (
        BUILD_DIR="$(mktemp -d "$work_dir/build.XXXXXX")/output"
        RAGFLOW_CMAKE_ARGS="$extra_args"
        unset CC CXX CPPFLAGS CFLAGS CXXFLAGS LDFLAGS LIBRARY_PATH CPATH

        uname() { [ "$1" = -s ] && printf '%s\n' "$os"; }
        go() { [ "$*" = 'env GOARCH' ] && printf '%s\n' "$arch"; }
        clang++() { :; }
        pkg-config() { :; }
        cmake() {
            printf '%s\n' "$@" > "$args_file"
            exit 0
        }

        main "$mode"
    )

    expected="$(printf '%s\n' .. -DCMAKE_BUILD_TYPE=Release "$@")"
    actual="$(cat "$args_file")"
    if [ "$actual" = "$expected" ]; then
        printf 'PASS: %s\n' "$description"
    else
        printf 'FAIL: %s\nExpected CMake arguments:\n%s\nActual CMake arguments:\n%s\n' \
            "$description" "$expected" "$actual"
        failures=$((failures + 1))
    fi
}

assert_cmake_args 'Intel macOS uses x86_64' Darwin amd64 --all '' \
    -DCMAKE_OSX_ARCHITECTURES=x86_64
assert_cmake_args 'Apple Silicon keeps arm64' Darwin arm64 --cpp '' \
    -DCMAKE_OSX_ARCHITECTURES=arm64
assert_cmake_args 'Other nonempty GOARCH values pass through' Darwin 386 --cpp '' \
    -DCMAKE_OSX_ARCHITECTURES=386
assert_cmake_args 'Empty GOARCH omits the architecture flag' Darwin '' --cpp ''
assert_cmake_args 'Linux omits the macOS architecture flag' Linux amd64 --cpp ''
assert_cmake_args 'Explicit CMake arguments follow the derived flag' Darwin amd64 --run \
    '-DCMAKE_OSX_ARCHITECTURES=arm64 -DPCRE2_STATIC_LIB=/tmp/libpcre2-8.a' \
    -DCMAKE_OSX_ARCHITECTURES=x86_64 -DCMAKE_OSX_ARCHITECTURES=arm64 \
    -DPCRE2_STATIC_LIB=/tmp/libpcre2-8.a
assert_cmake_args 'Standalone C++ test fallback uses its default arguments' Darwin amd64 --cpp-test ''

[ "$failures" -eq 0 ]
