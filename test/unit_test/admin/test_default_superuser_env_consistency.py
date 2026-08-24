#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
"""Regression tests for #16876 (admin server env-var consistency).

The admin server surfaced ``DEFAULT_SUPERUSER_EMAIL`` as if it were
effective, but the bootstrap path hardcoded the value. The fix reads
both ``DEFAULT_SUPERUSER_EMAIL`` and ``DEFAULT_SUPERUSER_PASSWORD``
from the environment at module load and uses them as the single
source of truth for the bootstrap (``init_default_admin`` and
``check_admin``). The ``list envs`` endpoint also surfaces
``DEFAULT_SUPERUSER_PASSWORD`` the same way as the email so the
report matches the bootstrap.

These tests read ``admin/server/auth.py`` and
``admin/server/services.py`` as text and use ``ast`` to assert:

* Both env vars are read at module level (so the env-var default is
  captured once, not on every call).
* The two bootstrap entry points (``init_default_admin``,
  ``check_admin``) build their user dicts from the module-level
  constants, not from string literals.
* The pre-existing ``list envs`` block for ``DEFAULT_SUPERUSER_EMAIL``
  is still in place, and a new block for
  ``DEFAULT_SUPERUSER_PASSWORD`` is added that follows the same shape.
* The hardcoded ``"admin@ragflow.io"`` / ``"admin"`` literals appear
  only as the fallback default in the module-level ``os.getenv`` call,
  not in the bootstrap dicts.

``ast``-based testing is used because the surrounding admin/auth
module pulls in ``flask`` + ``quart`` + the full RAGFlow admin server
which would otherwise need a much heavier stubbing fixture.
"""

import ast
import pathlib


REPO_ROOT = pathlib.Path(__file__).resolve().parents[3]
AUTH_PATH = REPO_ROOT / "admin" / "server" / "auth.py"
SERVICES_PATH = REPO_ROOT / "admin" / "server" / "services.py"


def _load_module(path: pathlib.Path) -> ast.Module:
    return ast.parse(path.read_text(encoding="utf-8"))


def _get_module_constants(module: ast.Module) -> dict[str, ast.Constant | ast.Call]:
    """Return every top-level name binding whose value is a string
    constant or a single ``os.getenv(...)`` call. Returns ``{name: node}``."""
    out = {}
    for stmt in module.body:
        if not isinstance(stmt, ast.Assign):
            continue
        if len(stmt.targets) != 1 or not isinstance(stmt.targets[0], ast.Name):
            continue
        value = stmt.value
        if isinstance(value, ast.Constant) and isinstance(value.value, str):
            out[stmt.targets[0].id] = value
        elif isinstance(value, ast.Call) and ((isinstance(value.func, ast.Name) and value.func.id == "getenv") or (isinstance(value.func, ast.Attribute) and value.func.attr == "getenv")):
            # Capture the ``os.getenv("DEFAULT_SUPERUSER_EMAIL", "admin@ragflow.io")``
            # pattern. We'll check the args manually.
            out[stmt.targets[0].id] = value
    return out


def _call_string_args(call: ast.Call) -> list[str]:
    out = []
    for arg in call.args:
        if isinstance(arg, ast.Constant) and isinstance(arg.value, str):
            out.append(arg.value)
    return out


def test_auth_module_reads_default_superuser_env_vars():
    """``admin.server.auth`` reads both env vars at module load and
    exposes them as module-level constants so the bootstrap and
    ``list envs`` paths can share the resolved value."""
    module = _load_module(AUTH_PATH)
    consts = _get_module_constants(module)
    assert "_DEFAULT_SUPERUSER_EMAIL" in consts, "expected a module-level _DEFAULT_SUPERUSER_EMAIL constant; the bootstrap must read DEFAULT_SUPERUSER_EMAIL at import time"
    assert "_DEFAULT_SUPERUSER_PASSWORD" in consts, "expected a module-level _DEFAULT_SUPERUSER_PASSWORD constant; the bootstrap must read DEFAULT_SUPERUSER_PASSWORD at import time"
    email_node = consts["_DEFAULT_SUPERUSER_EMAIL"]
    assert isinstance(email_node, ast.Call), "_DEFAULT_SUPERUSER_EMAIL must be os.getenv(...)"
    assert _call_string_args(email_node) == [
        "DEFAULT_SUPERUSER_EMAIL",
        "admin@ragflow.io",
    ]
    password_node = consts["_DEFAULT_SUPERUSER_PASSWORD"]
    assert isinstance(password_node, ast.Call), "_DEFAULT_SUPERUSER_PASSWORD must be os.getenv(...)"
    assert _call_string_args(password_node) == [
        "DEFAULT_SUPERUSER_PASSWORD",
        "admin",
    ]


def _find_function(module: ast.Module, name: str) -> ast.FunctionDef:
    for stmt in module.body:
        if isinstance(stmt, ast.FunctionDef) and stmt.name == name:
            return stmt
    raise AssertionError(f"function {name!r} not found")


def _resolve_value_to_name(node: ast.AST) -> str | None:
    """Return the bound name for ``node`` if it is a ``Name`` or
    a single-argument call whose argument is a ``Name``. The
    bootstrap dicts call ``encode_to_base64(_DEFAULT_SUPERUSER_PASSWORD)``
    so the dict value is a ``Call`` wrapping a ``Name`` rather than
    a bare ``Name``."""
    if isinstance(node, ast.Name):
        return node.id
    if isinstance(node, ast.Call) and node.args and isinstance(node.args[0], ast.Name):
        return node.args[0].id
    return None


def _literal_strings(node: ast.AST) -> list[str]:
    """All string literals found anywhere under ``node``. Used to
    guard against the regression where the bootstrap dict re-introduces
    the hardcoded ``"admin@ragflow.io"`` / ``"admin"`` strings."""
    out = []
    for sub in ast.walk(node):
        if isinstance(sub, ast.Constant) and isinstance(sub.value, str):
            out.append(sub.value)
    return out


def test_init_default_admin_uses_module_constants_not_string_literals():
    """The user dict built by ``init_default_admin`` must reference
    the module-level constants for both email and password. The
    hardcoded defaults only appear in the module-level ``os.getenv``
    call."""
    module = _load_module(AUTH_PATH)
    func = _find_function(module, "init_default_admin")

    # The email/password fields of the dict literal must be Name
    # nodes bound to the module constants — not string literals.
    dict_node = next(
        (n.value for n in ast.walk(func) if isinstance(n, ast.Assign) for tgt in n.targets if isinstance(tgt, ast.Name) and tgt.id == "default_admin" for n in [n] if isinstance(n.value, ast.Dict)),
        None,
    )
    assert dict_node is not None, "init_default_admin must assign a dict literal to `default_admin`"
    keys = [_literal_strings(k)[0] for k in dict_node.keys if _literal_strings(k)]
    assert "email" in keys and "password" in keys, f"init_default_admin dict must have email and password keys, found {keys!r}"
    email_value = next(v for k, v in zip(dict_node.keys, dict_node.values) if _literal_strings(k) == ["email"])
    password_value = next(v for k, v in zip(dict_node.keys, dict_node.values) if _literal_strings(k) == ["password"])
    assert _resolve_value_to_name(email_value) == "_DEFAULT_SUPERUSER_EMAIL", (
        f"default_admin['email'] must reference the module constant; got AST node {ast.dump(email_value) if email_value else 'None'}"
    )
    assert _resolve_value_to_name(password_value) == "_DEFAULT_SUPERUSER_PASSWORD", (
        f"default_admin['password'] must reference the module constant (directly or as the argument to encode_to_base64); got AST node {ast.dump(password_value) if password_value else 'None'}"
    )


def test_check_admin_uses_module_constants_for_fallback_user_info():
    """``check_admin`` builds its fallback ``user_info`` from the
    module constants so the user is auto-registered with the
    configured credentials, not the hardcoded pair."""
    module = _load_module(AUTH_PATH)
    func = _find_function(module, "check_admin")

    # The email/password fields of the fallback dict must be Name
    # nodes bound to the module constants.
    info_dicts = [
        n.value for n in ast.walk(func) if isinstance(n, ast.Assign) for tgt in n.targets if isinstance(tgt, ast.Name) and tgt.id == "user_info" for n in [n] if isinstance(n.value, ast.Dict)
    ]
    assert info_dicts, "check_admin must build a user_info dict literal"
    info_dict = info_dicts[0]
    keys = [_literal_strings(k)[0] for k in info_dict.keys if _literal_strings(k)]
    assert "email" in keys and "password" in keys
    email_value = next(v for k, v in zip(info_dict.keys, info_dict.values) if _literal_strings(k) == ["email"])
    password_value = next(v for k, v in zip(info_dict.keys, info_dict.values) if _literal_strings(k) == ["password"])
    assert _resolve_value_to_name(email_value) == "_DEFAULT_SUPERUSER_EMAIL"
    assert _resolve_value_to_name(password_value) == "_DEFAULT_SUPERUSER_PASSWORD"


def test_check_default_admin_lookup_uses_module_constant():
    """The ``[u for u in users if u.email == ...]`` lookup at the
    bottom of ``init_default_admin`` (used to attach a tenant to
    the existing default admin) must also use the env-driven
    module constant, not the hardcoded ``"admin@ragflow.io"``."""
    module = _load_module(AUTH_PATH)
    func = _find_function(module, "init_default_admin")

    string_literals = _literal_strings(func)
    assert "admin@ragflow.io" not in string_literals, "init_default_admin must not hardcode the admin email; the existing-admin lookup should use _DEFAULT_SUPERUSER_EMAIL"


def _list_envs_dicts(module: ast.Module) -> list[ast.Dict]:
    """Walk the module and return every dict literal whose keys
    include the string ``"env"`` (the ``list envs`` report format).
    Cheap parent-tracking-free because the report is the only place
    in ``admin/server/services.py`` that uses the ``"env"`` key."""
    out = []
    for d in ast.walk(module):
        if not isinstance(d, ast.Dict):
            continue
        keys = [k.value for k in d.keys if isinstance(k, ast.Constant) and isinstance(k.value, str)]
        if "env" in keys:
            out.append(d)
    return out


def test_list_envs_surfaces_default_superuser_email():
    """The pre-existing ``list envs`` block that surfaces
    ``DEFAULT_SUPERUSER_EMAIL`` is unchanged by this fix and must
    still be present so the operator-visible report matches the
    bootstrap."""
    module = _load_module(SERVICES_PATH)
    found = False
    for d in _list_envs_dicts(module):
        # The dict is shaped ``{"env": "<name>", "value": <expression>}``.
        # Walk ``values`` looking for the env-name string.
        for v in d.values:
            if isinstance(v, ast.Constant) and v.value == "DEFAULT_SUPERUSER_EMAIL":
                found = True
                break
        if found:
            break
    assert found, "list envs must still report DEFAULT_SUPERUSER_EMAIL"


def test_list_envs_surfaces_default_superuser_password_when_set():
    """When ``DEFAULT_SUPERUSER_PASSWORD`` is explicitly set, the
    ``list envs`` block must include the value so the operator can
    see whether the bootstrap honoured it. When the env var is
    unset the entry is intentionally omitted to avoid printing the
    well-known bootstrap default."""
    module = _load_module(SERVICES_PATH)
    has_password_entry = False
    for d in _list_envs_dicts(module):
        for v in d.values:
            if not (isinstance(v, ast.Name) and v.id == "default_superuser_password"):
                continue
            has_password_entry = True
            # The value must be a local variable bound to the os.getenv
            # call (so the env-var-only read matches the
            # env-var-only report) — not a hardcoded literal.
            # Find the binding.
            binding = next(
                (s for s in ast.walk(module) if isinstance(s, ast.Assign) and any(isinstance(t, ast.Name) and t.id == "default_superuser_password" for t in s.targets)),
                None,
            )
            assert binding is not None, "default_superuser_password must be bound to os.getenv(...) at the local scope"
            assert isinstance(binding.value, ast.Call), "default_superuser_password must be os.getenv(...)"
            args = _call_string_args(binding.value)
            assert args == ["DEFAULT_SUPERUSER_PASSWORD"], (
                f"os.getenv for the password must have exactly one arg (the env name); a second positional arg would re-introduce the well-known default. Got {args!r}"
            )
    assert has_password_entry, "list envs must surface DEFAULT_SUPERUSER_PASSWORD when the env var is set; see #16876"
