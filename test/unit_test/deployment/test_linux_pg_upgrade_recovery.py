"""Failure-inject actual upgrader control-flow slices without Docker or host writes."""

from pathlib import Path
import shutil
import subprocess
import unittest


ROOT = Path(__file__).resolve().parents[3]
SOURCE = (ROOT / "deployment/linux-pg/upgrade.sh").read_text(encoding="utf-8")


class UpgradeRecoveryTests(unittest.TestCase):
    def run_shell(self, body):
        bash = shutil.which("bash")
        self.assertIsNotNone(bash, "Bash is required to validate recovery; missing prerequisite is not a pass")
        harness = r"""
set -Eeuo pipefail
fixture=$(mktemp -d /tmp/ragflow-upgrade-test.XXXXXX)
trap 'cat "$fixture/commands"; rm -rf -- "$fixture"' EXIT
: > "$fixture/commands"
INSTALL_DIR=$fixture/current
PREVIOUS_DIR=$fixture/previous
FAILED_DIR=$fixture/failed
BACKUP_DIR=$fixture/backup
SECRETS_DIR=$fixture/secrets
STAGE_DIR=$fixture/stage
RECOVERY_REQUIRED_FILE=$SECRETS_DIR/upgrade-recovery-required.env
CURRENT_ENV=$fixture/env
RELEASE_VERSION=v9.0.0
mkdir -p "$INSTALL_DIR" "$PREVIOUS_DIR" "$BACKUP_DIR" "$SECRETS_DIR"
APP_STOPPED=0
SWITCHED=0
DATA_MAY_HAVE_CHANGED=0
die() { echo "$*" >&2; exit 1; }
env_value() { echo fixture; }
compose() {
  echo "compose $*" >> "$fixture/commands"
  if [[ ${2:-} == stop && ${FAIL_STOP:-0} == 1 ]]; then
    return 17
  fi
  if [[ ${2:-} == ps ]]; then
    [[ ${4:-} != "${MISSING_SERVICE:-none}" ]] || return 0
    echo "fixture-${4:-app}"
  fi
}
sudo() {
  echo "sudo $*" >> "$fixture/commands"
  case $1 in
    docker) echo sha256:fixture ;;
    mv) [[ ${FAIL_MV:-0} != 1 ]] && command mv "${@:2}" ;;
    tee|chmod|install) command "$@" ;;
    *) echo "Forbidden fixture command: $*" >&2; return 99 ;;
  esac
}
"""
        result = subprocess.run([bash, "-c", "bash -s"], input=(harness + "\n" + body).encode(), cwd=ROOT, capture_output=True, timeout=30)
        result.stdout = result.stdout.decode("utf-8", errors="replace")
        result.stderr = result.stderr.decode("utf-8", errors="replace")
        return result

    def handler(self):
        return SOURCE[SOURCE.index("upgrade_error() {") : SOURCE.index("trap upgrade_error ERR") + len("trap upgrade_error ERR")]

    def test_preflight_storage_missing_never_stops_application(self):
        start = SOURCE.index("# Resolve backup dependencies")
        end = SOURCE.index('if [[ ${CHECK_ONLY} == "1" ]]', start)
        result = self.run_shell("MISSING_SERVICE=postgres\n" + SOURCE[start:end])
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertIn("PostgreSQL container is not running", result.stderr)
        self.assertNotIn(" stop ", result.stdout)
        self.assertNotIn("sudo mv", result.stdout)

    def test_recovery_marker_blocks_before_version_noop(self):
        marker_guard = next(line for line in SOURCE.splitlines() if line.startswith("[[ ! -e ${RECOVERY_REQUIRED_FILE} ]]"))
        self.assertLess(SOURCE.index(marker_guard), SOURCE.index("FROM_VERSION=unknown"))
        result = self.run_shell(': > "$RECOVERY_REQUIRED_FILE"\n' + marker_guard + "\necho SHOULD_NOT_RUN\n")
        self.assertEqual(result.returncode, 1)
        self.assertIn("automatic retry is blocked", result.stderr)
        self.assertNotIn("SHOULD_NOT_RUN", result.stdout)
        self.assertNotIn("compose", result.stdout)

    def test_missing_application_never_attempts_stop_or_restart(self):
        start = SOURCE.index("# Resolve backup dependencies")
        end = SOURCE.index('if [[ ${CHECK_ONLY} == "1" ]]', start)
        result = self.run_shell("MISSING_SERVICE=ragflow-cpu\n" + SOURCE[start:end])
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertIn("RAGFlow container is not running before upgrade", result.stderr)
        self.assertNotIn(" stop ", result.stdout)
        self.assertNotIn(" start ", result.stdout)
        self.assertNotIn("sudo mv", result.stdout)

    def test_pre_boundary_backup_failure_restarts_existing_application(self):
        result = self.run_shell(self.handler() + "\nAPP_STOPPED=1\nfalse\n")
        self.assertEqual(result.returncode, 1)
        self.assertIn("start ragflow-cpu", result.stdout)
        self.assertNotIn("up -d", result.stdout)
        self.assertNotIn("sudo mv", result.stdout)

    def test_partial_stop_failure_restarts_existing_application(self):
        start = SOURCE.index("upgrade_fail() {")
        end = SOURCE.index('sudo install -m 0600 "${CURRENT_ENV}"', start)
        result = self.run_shell(self.handler() + "\nFAIL_STOP=1\n" + SOURCE[start:end])
        self.assertEqual(result.returncode, 17, result.stderr)
        self.assertIn("stop ragflow-cpu", result.stdout)
        self.assertIn("start ragflow-cpu", result.stdout)
        self.assertNotIn("up -d", result.stdout)
        self.assertNotIn("sudo mv", result.stdout)

    def test_post_boundary_failure_preserves_both_releases_without_restart(self):
        result = self.run_shell(self.handler() + "\nAPP_STOPPED=1\nSWITCHED=1\nDATA_MAY_HAVE_CHANGED=1\nfalse\n")
        self.assertEqual(result.returncode, 1)
        self.assertIn("Automatic code rollback is prohibited", result.stderr)
        self.assertIn("stop ragflow-cpu", result.stdout)
        self.assertNotIn("up -d", result.stdout)
        self.assertNotIn("start ragflow-cpu", result.stdout)
        self.assertNotIn("sudo mv", result.stdout)

    def test_pre_boundary_failed_directory_restore_never_starts_wrong_tree(self):
        result = self.run_shell(self.handler() + "\nAPP_STOPPED=1\nSWITCHED=1\nFAIL_MV=1\nfalse\n")
        self.assertEqual(result.returncode, 1)
        self.assertIn("sudo mv", result.stdout)
        self.assertNotIn("up -d", result.stdout)
        self.assertNotIn("start ragflow-cpu", result.stdout)

    def test_pre_boundary_successful_directory_restore_only_starts_existing_app(self):
        result = self.run_shell(self.handler() + "\nAPP_STOPPED=1\nSWITCHED=1\nfalse\n")
        self.assertEqual(result.returncode, 1)
        self.assertIn("sudo mv", result.stdout)
        self.assertIn("start ragflow-cpu", result.stdout)
        self.assertNotIn("up -d", result.stdout)

    def test_persist_marker_and_boundary_before_first_new_process(self):
        start = SOURCE.index("# The entrypoint may migrate")
        end = SOURCE.index('wait_for_health "${BACKUP_DIR}/health-after.json"', start)
        result = self.run_shell(SOURCE[start:end] + '\ntest "$DATA_MAY_HAVE_CHANGED" = 1\ntest -s "$RECOVERY_REQUIRED_FILE"\n')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertLess(result.stdout.index("sudo tee"), result.stdout.index("up -d"))
        self.assertLess(SOURCE.index("DATA_MAY_HAVE_CHANGED=1", start), SOURCE.index('compose "${INSTALL_DIR}" up -d --no-build --pull never', start))

    def test_delivery_profiles_prepare_before_stop_and_never_build_after_switch(self):
        start = SOURCE.index('if [[ ${CHECK_ONLY} == "1" ]]')
        end = SOURCE.index("STAMP=$(date", start)
        block = SOURCE[start:end]
        for check, registry, offline, expected in ((1, 0, 0, None), (0, 1, 0, "pull"), (0, 0, 1, None), (0, 0, 0, "build")):
            with self.subTest(check=check, registry=registry, offline=offline):
                result = self.run_shell(f"CHECK_ONLY={check}\nREGISTRY_INSTALL={registry}\nOFFLINE_INSTALL={offline}\nFROM_VERSION=v8.0.0\nBACKUP_ROOT=$fixture\n" + block)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertNotIn(" stop ", result.stdout)
                if expected:
                    self.assertIn(" " + expected, result.stdout)
                else:
                    self.assertNotIn("compose", result.stdout)
        self.assertLess(start, SOURCE.index('APP_STOPPED=1\ncompose "${INSTALL_DIR}" stop ragflow-cpu'))
        self.assertNotIn("--build", SOURCE[SOURCE.index("SWITCHED=1\nsudo mv") :])


if __name__ == "__main__":
    unittest.main()
