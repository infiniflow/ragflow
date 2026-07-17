"""Standalone isolation check.

Run this file in the SAME pytest session as ``test_auth_env.py`` and
verify that ``sys.modules`` entries the auth tests installed
(``common``, ``api.*``, ``admin.*``) do not survive across tests, and
that the prior identity of ``admin.server.auth`` (whatever it was
before the auth tests ran) is exactly what is in ``sys.modules``
afterwards.

We rely on pytest's deterministic collection order: this file is
collected AFTER ``test_auth_env.py`` (alphabetical), so by the time
these tests run the auth tests have completed and torn down their
fixtures. The module-level ``_PRIOR_STATE`` snapshot captures the
``sys.modules`` state at collection time — *before* any tests in
``test_auth_env.py`` run — and the assertion functions below compare
the current ``sys.modules`` against it.

Only the names this conftest's stubs install are snapshotted, so an
unrelated test loading (say) ``common.scores`` does not pollute the
comparison.
"""

from __future__ import annotations

import sys


# ---------------------------------------------------------------------------
# Capture prior sys.modules state
# ---------------------------------------------------------------------------

_STUBBABLE_NAMES = (
    "common",
    "common.constants",
    "common.misc_utils",
    "common.time_utils",
    "common.connection_utils",
    "common.settings",
    "api",
    "api.common",
    "api.common.exceptions",
    "api.common.base64",
    "api.utils",
    "api.utils.crypt",
    "api.db",
    "api.db.services",
    "api.db.services.user_service",
    "admin",
    "admin.server",
    "admin.server.auth",
)

_PRIOR_STATE: dict[str, object | None] = {name: sys.modules.get(name) for name in _STUBBABLE_NAMES}


def _identity_restored(name: str) -> bool:
    """Return True iff ``sys.modules[name]`` is the same object as the
    snapshot taken at collection time (or both are absent)."""
    return _PRIOR_STATE.get(name) is sys.modules.get(name)


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------


def test_stub_modules_removed_from_sys_modules():
    """After the admin auth tests have run, every name the conftest's
    stub-install touched must point at the exact same object it did
    at collection time (or be absent, if it was absent then).

    The stubs we install are fresh ``types.ModuleType`` instances
    with no ``__loader__``; a real loaded module always carries one.
    If monkeypatch was bypassed for any of these names the identity
    check fails because the freshly imported stub has taken its
    place — even if the stub still happens to look like a real module
    by surface attributes alone, its ``id()`` would differ from the
    collection-time snapshot."""
    leaked = []
    for name in _STUBBABLE_NAMES:
        module = sys.modules.get(name)
        prior = _PRIOR_STATE.get(name)
        # Skip only when the module was absent at collection time *and*
        # is still absent. If a real module was loaded before the
        # tests ran and the conftest (or any side effect) has since
        # removed it, fall through so the deletion leak is reported
        # — never ``continue`` based solely on the current state.
        if module is None and prior is None:
            continue
        if module is None:
            leaked.append(f"{name} (deleted from sys.modules after the test suite started)")
            continue
        if getattr(module, "__loader__", None) is None:
            leaked.append(f"{name} (no __loader__ — stub leaked)")
        if module is not prior:
            leaked.append(f"{name} (identity differs from snapshot)")
    assert not leaked, f"admin auth tests leaked conftest stubs into sys.modules: {leaked}"


def test_admin_server_auth_identity_restored():
    """The conftest re-imports ``admin.server.auth`` for every auth
    test. If the import bypasses ``monkeypatch.setitem``, monkeypatch
    has no record of the change and cannot restore the prior identity
    on teardown — silently leaking the freshly imported module.

    The conftest now uses ``monkeypatch.setitem`` for both the stub
    modules and the imported ``admin.server.auth``; this test asserts
    that the prior identity (whatever it was at collection time — in
    practice, ``None`` because no other test in this directory imports
    ``admin.server.auth``) is restored. The identity check is
    stronger than a mere ``__loader__`` / ``__file__`` probe: even if
    the leaked module were somehow indistinguishable from a real one,
    its ``id()`` would still differ from the snapshot taken before
    the auth tests ran."""
    assert _identity_restored("admin.server.auth"), (
        "admin.server.auth in sys.modules is not the same object as the snapshot taken before the auth tests ran — the conftest bypassed monkeypatch and leaked a freshly imported module"
    )
