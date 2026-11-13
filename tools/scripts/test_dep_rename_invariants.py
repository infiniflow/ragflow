"""Guard the two intentional exceptions of the ragflow_deps -> deps rename.

The rename PR decided to keep (a) the published image tag `infiniflow/ragflow_deps:latest`
and (b) the CI install path `/opt/ragflow_deps` unchanged. A blanket
`sed s/ragflow_deps/deps/g` accidentally rewrote those too, breaking the CI
offline fallback for the cl100k table and leaving misleading tag references.

This test fails while those exceptions are violated, and stays green once the
intentional names are restored. Run standalone: `python3 tools/scripts/test_dep_rename_invariants.py`.
"""

import os
import sys

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

SEP_TESTS = os.path.join(REPO_ROOT, ".github", "workflows", "sep-tests.yml")
DOCKERIGNORE = os.path.join(REPO_ROOT, ".dockerignore")
DOCKERFILE = os.path.join(REPO_ROOT, "Dockerfile")
DOCKERFILE_BASE = os.path.join(REPO_ROOT, "Dockerfile_base")

# The bare, wrong forms that the over-broad rename introduced.
BARE_OPT_DEPS = "/opt/deps"  # must stay /opt/ragflow_deps (CI install path)
BARE_IMAGE_TAG = "infiniflow/deps"  # must stay infiniflow/ragflow_deps (image tag)

# The intentional, retained forms that must remain present.
KEPT_OPT_DEPS = "/opt/ragflow_deps"
KEPT_IMAGE_TAG = "infiniflow/ragflow_deps"


def _read(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()


def test_sep_tests_keeps_opt_ragflow_deps_path():
    text = _read(SEP_TESTS)
    # The CI install path must NOT have been renamed to /opt/deps.
    assert BARE_OPT_DEPS not in text, f"{SEP_TESTS} still references '{BARE_OPT_DEPS}'; the CI install path must remain '{KEPT_OPT_DEPS}' (intentional rename exception)."
    # The retained path must actually be present as the cl100k source.
    assert KEPT_OPT_DEPS in text, f"{SEP_TESTS} is missing '{KEPT_OPT_DEPS}'; the CI cl100k source was lost."


def test_sep_tests_keeps_ragflow_deps_image_tag():
    text = _read(SEP_TESTS)
    # Comments/echo strings must reference the real tag, not the bare wrong one.
    assert BARE_IMAGE_TAG not in text, f"{SEP_TESTS} still references bare '{BARE_IMAGE_TAG}'; the published image tag is '{KEPT_IMAGE_TAG}'."
    assert KEPT_IMAGE_TAG in text, f"{SEP_TESTS} is missing '{KEPT_IMAGE_TAG}'; the image tag reference was lost."


def test_dockerignore_keeps_ragflow_deps_image_tag():
    text = _read(DOCKERIGNORE)
    assert BARE_IMAGE_TAG not in text, f"{DOCKERIGNORE} still references bare '{BARE_IMAGE_TAG}'; the published image tag is '{KEPT_IMAGE_TAG}'."
    assert KEPT_IMAGE_TAG in text, f"{DOCKERIGNORE} is missing '{KEPT_IMAGE_TAG}'; the image tag reference was lost."


def test_dockerfiles_keep_ragflow_deps_image_tag():
    for path in (DOCKERFILE, DOCKERFILE_BASE):
        text = _read(path)
        # Error-message echo strings must reference the real tag, not the bare wrong one.
        assert BARE_IMAGE_TAG not in text, f"{path} still references bare '{BARE_IMAGE_TAG}'; the published image tag is '{KEPT_IMAGE_TAG}'."
        # The mount source (infiniflow/ragflow_deps:latest) must be present.
        assert KEPT_IMAGE_TAG in text, f"{path} is missing '{KEPT_IMAGE_TAG}'; the image tag reference was lost."


def main():
    tests = [
        test_sep_tests_keeps_opt_ragflow_deps_path,
        test_sep_tests_keeps_ragflow_deps_image_tag,
        test_dockerignore_keeps_ragflow_deps_image_tag,
        test_dockerfiles_keep_ragflow_deps_image_tag,
    ]
    failed = 0
    for t in tests:
        try:
            t()
            print(f"PASS  {t.__name__}")
        except AssertionError as e:
            failed += 1
            print(f"FAIL  {t.__name__}: {e}")
    if failed:
        print(f"\n{failed} test(s) failed.")
        return 1
    print("\nAll rename-invariant checks passed.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
