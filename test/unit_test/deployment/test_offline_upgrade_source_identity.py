"""Exercise offline extraction before preflight without Docker or shared data."""

from pathlib import Path
import shutil
import subprocess

import pytest


ROOT = Path(__file__).resolve().parents[3]
SOURCE = (ROOT / "deployment/linux-pg/upgrade_offline.sh").read_text(encoding="utf-8")


@pytest.mark.parametrize("existing", [True, False])
def test_offline_source_must_be_fresh_before_preflight_or_image_load(existing):
    bash = shutil.which("bash")
    assert bash, "Bash prerequisite missing; offline extraction validation incomplete"
    start = SOURCE.index('if sudo test -e "${SOURCE_DIR}"')
    end = SOURCE.index("run_preflight()", start)
    assert end < SOURCE.index("sudo docker load -i")
    harness = r"""
set -Eeuo pipefail
fixture=$(mktemp -d /tmp/ragflow-offline-identity.XXXXXX)
trap 'cat "$fixture/commands"; printf "MANIFEST="; cat "$SOURCE_DIR/DEPLOYMENT-CANDIDATE.json"; rm -rf -- "$fixture"' EXIT
SOURCE_DIR=$fixture/source
PAYLOAD_DIR=$fixture/payload
SOURCE_ARCHIVE=source.tar.gz
FRONTEND_ARCHIVE=frontend.tar.gz
RELEASE_VERSION=v1.2.3
mkdir -p "$SOURCE_DIR" "$PAYLOAD_DIR" "$fixture/source-b" "$fixture/frontend-b/dist"
: > "$fixture/commands"
printf 'RELEASE_VERSION=v1.2.3\n' > "$fixture/source-b/DEPLOYMENT-SOURCE.env"
printf 'candidate-B\n' > "$fixture/source-b/DEPLOYMENT-CANDIDATE.json"
printf 'frontend-B\n' > "$fixture/frontend-b/dist/index.html"
tar -czf "$PAYLOAD_DIR/$SOURCE_ARCHIVE" -C "$fixture/source-b" .
tar -czf "$PAYLOAD_DIR/$FRONTEND_ARCHIVE" -C "$fixture/frontend-b" dist
sudo() {
  echo "sudo $*" >> "$fixture/commands"
  case $1 in
    test|find|install|tar) command "$@" ;;
    *) echo 'Forbidden fixture operation' >&2; return 99 ;;
  esac
}
"""
    if existing:
        harness += r"""
mkdir -p "$SOURCE_DIR/web/dist"
printf 'RELEASE_VERSION=v1.2.3\n' > "$SOURCE_DIR/DEPLOYMENT-SOURCE.env"
printf 'candidate-A\n' > "$SOURCE_DIR/DEPLOYMENT-CANDIDATE.json"
printf 'frontend-A\n' > "$SOURCE_DIR/web/dist/index.html"
"""
    result = subprocess.run(
        [bash, "-c", "bash -s"],
        input=(harness + SOURCE[start:end] + "\necho PREFLIGHT_REACHED\n").encode(),
        capture_output=True,
        timeout=30,
    )
    output = result.stdout.decode()
    if existing:
        assert result.returncode == 1
        assert "Select a fresh SOURCE_DIR" in result.stderr.decode()
        assert "MANIFEST=candidate-A" in output
        assert "sudo tar" not in output
        assert "PREFLIGHT_REACHED" not in output
    else:
        assert result.returncode == 0, result.stderr.decode()
        assert "MANIFEST=candidate-B" in output
        assert output.count("sudo tar -xzf") == 2
        assert "PREFLIGHT_REACHED" in output
