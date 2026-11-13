#!/usr/bin/env bash
# TDD test for the pdf_oxide native-lib version gate in build.sh.
#
# The version marker is embedded in Rust's string constant pool as
# "pdf_oxide <version>" and cannot be anchored to a fixed length: a suffixed
# build (0.3.73.1) and a clean 0.3.73 whose next pooled constant begins with a
# digit are byte-identical. So we capture exactly three dotted segments and
# treat any trailing [0-9.] as ambiguous rather than silently truncating it
# into a (wrong) match.
#
# Run: bash test/build_pdf_oxide_version.sh
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../build.sh
source "${SCRIPT_DIR}/../build.sh"
# build.sh sets 'set -e'; the test must control its own flow.
set +e

failures=0

# assert_status <desc> <expected_rc> <actual_rc> [<expected_out>] [<actual_out>]
assert_status() {
    local desc="$1" expected_rc="$2" actual_rc="$3" expected_out="${4:-}" actual_out="${5:-}"
    if [ "$expected_rc" != "$actual_rc" ]; then
        echo "FAIL: $desc (expected rc=$expected_rc, got rc=$actual_rc)"
        failures=$((failures + 1))
        return
    fi
    if [ -n "$expected_out" ] && [ "$expected_out" != "$actual_out" ]; then
        echo "FAIL: $desc (expected out=[$expected_out], got out=[$actual_out])"
        failures=$((failures + 1))
        return
    fi
    echo "PASS: $desc"
}

# Helper that runs pdf_oxide_validate_version and captures rc + stdout.
run_validate() {
    local pool="$1" required="$2" out rc
    out=$(pdf_oxide_validate_version "$pool" "$required"); rc=$?
    printf '%s\t%s' "$rc" "$out"
}

# 1. Exact three-segment match passes (rc 0, prints version).
read -r rc out < <(run_validate "pdf_oxide 0.3.73ForPublicRelease" "0.3.73")
assert_status "exact 0.3.73 match" 0 "$rc" "0.3.73" "$out"

# 2. Suffixed build 0.3.73.1 is ambiguous (rc 2), never silently accepted.
read -r rc out < <(run_validate "pdf_oxide 0.3.73.1" "0.3.73")
assert_status "suffix 0.3.73.1 is ambiguous" 2 "$rc"

# 3. A version with an extra appended digit (0.3.731) must be rejected as a
#    mismatch (rc 1), NOT silently accepted as 0.3.73.
read -r rc out < <(run_validate "pdf_oxide 0.3.731foobar" "0.3.73")
assert_status "digit-appended 0.3.731 mismatch" 1 "$rc"

# 4. Truncated pin 0.3.7 must mismatch against a 0.3.73 lib (rc 1).
read -r rc out < <(run_validate "pdf_oxide 0.3.73" "0.3.7")
assert_status "truncated pin 0.3.7 mismatch" 1 "$rc"

# 5. Wrong three-segment version mismatches (rc 1).
read -r rc out < <(run_validate "pdf_oxide 0.3.72" "0.3.73")
assert_status "wrong version 0.3.72 mismatch" 1 "$rc"

# 6. No version marker at all mismatches (rc 1).
read -r rc out < <(run_validate "unrelated string pool" "0.3.73")
assert_status "missing marker mismatch" 1 "$rc"

if [ "$failures" -eq 0 ]; then
    echo "ALL TESTS PASSED"
    exit 0
else
    echo "$failures TEST(S) FAILED"
    exit 1
fi
