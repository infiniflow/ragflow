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
"""Regression tests for #17578 (mysql_migration gaps).

The v0.25.5 -> v0.26.4 migration of ``tenant_llm`` to
``tenant_model_instance`` had three gaps:

* ``api_base`` was dropped — the runtime reads ``base_url`` from
  ``extra`` so every embedding/chat call failed with
  "url cannot be None".
* ``instance_name`` was hardcoded to ``"default"`` — multiple
  api_keys for the same provider collided on the same
  ``instance_name`` and the lookup
  ``Instance default not found`` errored out.
* (The third gap, model_type mapping, is handled by
  ``migrate_model_type_names()`` in ``api/db/db_models.py``; the
  reporter's framing was slightly off but the existing rename is
  correct.)

This test loads the migration module with the heavy
peewee / MySQL machinery stubbed and asserts the prepared INSERT
statements carry the right ``extra`` payload and per-(tenant,
provider) ``instance_name``. We exercise the ``execute`` method
with a mock DB so we can inspect the generated SQL without a real
MySQL instance.
"""

import importlib.util
import os
import re
import sys
from types import ModuleType
from unittest.mock import MagicMock


REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))


def _stub_module(name, **attrs):
    module = ModuleType(name)
    for key, value in attrs.items():
        setattr(module, key, value)
    sys.modules[name] = module
    return module


def _load_migration(monkeypatch):
    """Load ``tools/scripts/mysql_migration.py`` with the heavy
    MySQL / peewee / packaging machinery stubbed. Returns the module
    object so individual tests can grab the stage classes and mock
    the DB.
    """
    snapshot = {
        name: sys.modules.get(name)
        for name in (
            "peewee",
            "playhouse",
            "playhouse.migrate",
            "packaging",
            "packaging.version",
        )
    }
    try:
        peewee_stub = _stub_module("peewee")
        for name in (
            "BigIntegerField",
            "CharField",
            "DateTimeField",
            "IntegerField",
            "Model",
            "MySQLDatabase",
            "PrimaryKeyField",
            "TextField",
        ):
            setattr(peewee_stub, name, MagicMock())
        _stub_module("playhouse")
        _stub_module("playhouse.migrate", MySQLMigrator=MagicMock())
        _stub_module("packaging")
        _stub_module("packaging.version", InvalidVersion=Exception, Version=MagicMock())

        module_path = os.path.join(REPO_ROOT, "tools", "scripts", "mysql_migration.py")
        spec = importlib.util.spec_from_file_location("_mysql_migration_under_test", module_path)
        module = importlib.util.module_from_spec(spec)
        sys.modules["_mysql_migration_under_test"] = module
        spec.loader.exec_module(module)
        return module
    finally:
        for name, original in snapshot.items():
            if original is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = original


def _make_stage(migration_module):
    """Construct a ``TenantModelInstanceStage`` with a mock DB.

    The stage is the one that runs the per-tenant INSERT into
    ``tenant_model_instance``. We mock the DB so we can capture
    the generated SQL and the records passed in.
    """
    stage = migration_module.TenantModelInstanceStage.__new__(migration_module.TenantModelInstanceStage)
    stage.db = MagicMock()
    # Have the lookup query return a deterministic batch of records.
    return stage


def _run_execute(stage, dedup_rows, full_rows):
    """Wire ``stage.db.execute_sql`` to return ``dedup_rows`` for the
    first SELECT (used by the dedup helper) and ``full_rows`` for the
    second SELECT (which carries ``api_base`` and ``llm_name``), then
    capture the generated INSERT SQL and return it.

    ``dedup_rows`` is a list of 5-tuples ``(tenant_id, llm_factory,
    api_key, status, provider_id)`` matching the first SELECT in
    ``execute``. ``full_rows`` is a list of 7-tuples ``(tenant_id,
    llm_factory, api_key, status, provider_id, api_base, llm_name)``
    matching the second SELECT.
    """
    dedup_cursor = MagicMock()
    dedup_cursor.fetchall.return_value = dedup_rows
    full_cursor = MagicMock()
    full_cursor.fetchall.return_value = full_rows

    # Call order:
    # 1. SELECT (5-tuples) -> dedup_cursor
    # 2. dedup re-fetch SELECT (7-tuples) -> full_cursor
    # 3. INSERT -> None
    # All other execute_sql calls (table_exists checks, etc.) get
    # a MagicMock that returns a truthy default for ``rowcount`` /
    # ``fetchone`` style access.
    def _side_effect(*args, **kwargs):
        call_index = _run_execute.counter
        _run_execute.counter += 1
        if call_index == 0:
            return dedup_cursor
        if call_index == 1:
            return full_cursor
        return None

    _run_execute.counter = 0
    stage.db.execute_sql.side_effect = _side_effect
    stage.dry_run = False
    stage.create_table_only = False
    stage.create_target_table = MagicMock()
    stage.execute()
    insert_call = stage.db.execute_sql.call_args_list[-1]
    return insert_call.args[0]


def test_migration_passes_api_base_into_extra_column(monkeypatch):
    """Gap 1: ``tenant_llm.api_base`` must end up as
    ``extra.base_url`` in the new ``tenant_model_instance`` row.
    Without this, the runtime reads ``base_url=None`` and every
    call fails with ``url cannot be None``."""
    module = _load_migration(monkeypatch)
    stage = _make_stage(module)

    row = (
        "tenant-1",
        "OpenAI-API-Compatible",
        "sk-abc",
        "1",
        "provider-1",
        "http://gateway.local:3001/v1",
        "gpt-4",
    )
    insert_sql = _run_execute(
        stage,
        dedup_rows=[row[:5]],
        full_rows=[row],
    )

    # The INSERT must include the ``extra`` column. The current bug
    # is that the column is missing entirely from the column list.
    assert "(id, instance_name, provider_id, api_key, status," in insert_sql
    assert "extra" in insert_sql.split("VALUES", 1)[0]
    # The ``api_base`` value must be embedded as JSON in the extra
    # column.
    match = re.search(r"VALUES\s*\((.*?)\)\s*$", insert_sql, flags=re.DOTALL)
    assert match, f"could not find VALUES clause in {insert_sql!r}"
    values_clause = match.group(1)
    # The extra JSON appears as a SQL string literal: '{\\"base_url\\": \\"http://gateway.local:3001/v1\\"}'.
    assert "base_url" in values_clause
    assert "http://gateway.local:3001/v1" in values_clause


def test_migration_writes_empty_extra_when_api_base_is_null(monkeypatch):
    """When ``tenant_llm.api_base`` is NULL or empty (e.g. OpenAI's
    default endpoint), the extra column must be the empty JSON
    object ``{}`` — not NULL, not a string, not a leftover from
    the previous batch."""
    module = _load_migration(monkeypatch)
    stage = _make_stage(module)

    rows = [
        ("tenant-1", "OpenAI", "sk-abc", "1", "provider-1", None, "gpt-4"),
        ("tenant-1", "OpenAI", "sk-def", "1", "provider-1", "", "gpt-4-turbo"),
    ]
    insert_sql = _run_execute(
        stage,
        dedup_rows=[r[:5] for r in rows],
        full_rows=rows,
    )

    match = re.search(r"VALUES\s*(.*?)\)\s*$", insert_sql, flags=re.DOTALL)
    assert match
    values_section = match.group(1)
    # The two value tuples must both carry ``extra`` = '{}'. Look
    # for the literal '{}' surrounded by single-quote SQL string
    # delimiters.
    empty_extra_occurrences = re.findall(r"'(?:\{\}|\{\s*\})'", values_section)
    assert len(empty_extra_occurrences) == 2, f"expected 2 empty-extra literals, got {len(empty_extra_occurrences)} in {values_section!r}"


def _instance_names_from_insert(insert_sql: str) -> list[str]:
    """Parse the generated INSERT and return the second SQL literal
    from each VALUES tuple — that's ``instance_name`` (the column
    list is ``(id, instance_name, provider_id, api_key, status, extra, ...)``).

    The generated INSERT may have several rows separated by ``,`` and
    each row has nested function calls (``FROM_UNIXTIME(12345)``) whose
    ``)`` is indistinguishable from a tuple boundary to a naive regex.
    Walk the values clause character-by-character to count balanced
    parens (treating ``''`` inside string literals as an escaped quote
    rather than an open/close pair) and split at the top-level tuple
    boundaries only.
    """
    # Grab the whole VALUES clause (greedy, no trailing-`)` anchor).
    # The walk below handles the final ``)`` of the last tuple as a
    # normal depth-0 close. A non-greedy regex would over-match into
    # the ``)`` of the first ``FROM_UNIXTIME(...)`` call, so use
    # ``re.DOTALL`` to let ``.*`` span newlines.
    match = re.search(r"VALUES\s*(.*)\Z", insert_sql, flags=re.DOTALL)
    assert match, f"could not find VALUES clause in {insert_sql!r}"
    values_section = match.group(1).rstrip()
    # Trim a trailing comma if the migration emitted one (defensive —
    # the current migration does not).
    values_section = values_section.rstrip(",")

    tuples: list[str] = []
    depth = 0
    current_start: int | None = None
    in_single_quote = False
    i = 0
    n = len(values_section)
    while i < n:
        c = values_section[i]
        if c == "'":
            # ``''`` is a SQL-escaped single quote. When we are inside
            # a string, the pair represents a literal ``'`` and we stay
            # in the string (no state change). When we are outside a
            # string, ``''`` is the empty string literal — also no net
            # state change after the close. Either way, skip both
            # characters and leave ``in_single_quote`` untouched.
            if i + 1 < n and values_section[i + 1] == "'":
                i += 2
                continue
            in_single_quote = not in_single_quote
            i += 1
            continue
        if in_single_quote:
            i += 1
            continue
        if c == "(":
            if depth == 0:
                current_start = i
            depth += 1
        elif c == ")":
            depth -= 1
            if depth == 0 and current_start is not None:
                tuples.append(values_section[current_start : i + 1])
                current_start = None
        i += 1
    assert tuples, f"could not find any tuples in {values_section!r}"

    names: list[str] = []
    for t in tuples:
        literals = re.findall(r"'((?:[^'\\]|\\.)*)'", t)
        # The tuple is (id, instance_name, provider_id, api_key, status, extra, ...).
        # literal 0 is the UUID id, literal 1 is instance_name.
        assert len(literals) >= 2, f"tuple has < 2 literals: {t!r}"
        names.append(literals[1])
    return names


def test_migration_generates_unique_instance_name_per_provider(monkeypatch):
    """Gap 3: multiple api_keys for the same provider must NOT
    collide on ``instance_name``. First instance keeps the
    ``llm_name`` (or ``default`` fallback); further instances get a
    ``-1``, ``-2``... suffix within the same
    (tenant_id, provider_id) group."""
    module = _load_migration(monkeypatch)
    stage = _make_stage(module)

    rows = [
        ("tenant-1", "OpenAI", "sk-a", "1", "prov-1", None, "gpt-4"),
        ("tenant-1", "OpenAI", "sk-b", "1", "prov-1", None, "gpt-4-turbo"),
        ("tenant-1", "OpenAI", "sk-c", "1", "prov-1", None, "gpt-3.5-turbo"),
        # Different tenant / different provider, should restart the
        # counter and use the per-record llm_name.
        ("tenant-2", "OpenAI", "sk-d", "1", "prov-2", None, "gpt-4"),
    ]
    insert_sql = _run_execute(
        stage,
        dedup_rows=[r[:5] for r in rows],
        full_rows=rows,
    )

    instance_names = _instance_names_from_insert(insert_sql)
    assert instance_names == ["gpt-4", "gpt-4-turbo", "gpt-3.5-turbo", "gpt-4"], f"expected unique instance_names per (tenant, provider); got {instance_names!r}"
    # The fourth record is a fresh (tenant, provider) group, so the
    # counter restarts at 0 and the llm_name is used as-is.
    assert instance_names[3] == "gpt-4"


def test_migration_instance_name_falls_back_to_default_when_llm_name_blank(monkeypatch):
    """When ``tenant_llm.llm_name`` is NULL or empty the instance
    name falls back to ``default`` (the historical hardcoded
    value), with the same per-(tenant, provider) counter suffix as
    any other naming source."""
    module = _load_migration(monkeypatch)
    stage = _make_stage(module)

    rows = [
        # Distinct api_keys so the dedup helper keeps both rows. The
        # ``llm_name`` is NULL/empty for both, so both fall back to
        # ``default`` and the counter keeps them distinct.
        ("tenant-1", "OpenAI", "sk-a-unique-1", "1", "prov-1", None, None),
        ("tenant-1", "OpenAI", "sk-b-unique-2", "1", "prov-1", None, ""),
    ]
    insert_sql = _run_execute(
        stage,
        dedup_rows=[r[:5] for r in rows],
        full_rows=rows,
    )

    instance_names = _instance_names_from_insert(insert_sql)
    # Both rows fall back to ``default`` because llm_name is blank.
    # The counter keeps them distinct: ``default`` and ``default-1``.
    assert instance_names == ["default", "default-1"], f"expected default fallback with counter suffix; got {instance_names!r}"


def test_migration_sanitises_llm_name_for_safe_instance_name(monkeypatch):
    """``tenant_llm.llm_name`` may contain characters that are
    illegal or ugly in an instance_name (spaces, slashes, quote
    characters, non-ASCII). The migration sanitises non
    ``[A-Za-z0-9_.-]`` characters to ``_`` and clamps to 120 chars
    so the SQL string literal stays well-formed."""
    module = _load_migration(monkeypatch)
    stage = _make_stage(module)

    long_name = "a" * 200  # 200 chars, will be truncated
    rows = [("tenant-1", "OpenAI", "sk-a", "1", "prov-1", None, f"model with spaces/{long_name}!@#")]
    insert_sql = _run_execute(
        stage,
        dedup_rows=[r[:5] for r in rows],
        full_rows=rows,
    )

    instance_names = _instance_names_from_insert(insert_sql)
    assert len(instance_names) == 1
    instance_name = instance_names[0]
    # The sanitiser must replace spaces, slashes, punctuation with
    # underscores so the SQL string literal is well-formed and the
    # name is safe to use as an instance handle.
    assert " " not in instance_name
    assert "/" not in instance_name
    assert "!" not in instance_name
    assert "@" not in instance_name
    assert "#" not in instance_name
    # Clamp to 120 chars so the column type (VARCHAR(128)) is not
    # exceeded.
    assert len(instance_name) <= 120
    # The sanitised form is `"model_with_spaces_" + "a" * 200 + "_"`,
    # length 220. After the 120-char clamp, it is the first 120 chars
    # of that: `"model_with_spaces_"` (19 chars) + `"a" * 101`. The
    # final `_` that the `!@#` -> `_` replacement would produce is
    # past the clamp, so the name ends in `a`, not `_`.
    assert instance_name.startswith("model_with_spaces_")
    assert instance_name.endswith("a")
    assert instance_name[-1] != "_"


def test_migration_includes_all_columns_in_insert(monkeypatch):
    """The INSERT column list must include ``extra`` so the row
    actually carries the api_base. Pre-fix, the column list ended at
    ``update_date`` and ``extra`` defaulted to ``'{}'`` (which is
    why ``url cannot be None`` fired at runtime)."""
    module = _load_migration(monkeypatch)
    stage = _make_stage(module)

    rows = [("tenant-1", "OpenAI", "sk-a", "1", "prov-1", "http://gw/v1", "gpt-4")]
    insert_sql = _run_execute(
        stage,
        dedup_rows=[r[:5] for r in rows],
        full_rows=rows,
    )

    columns_section = insert_sql.split("VALUES", 1)[0]
    # The full column list with the new ``extra`` column.
    expected_columns = "(id, instance_name, provider_id, api_key, status, extra, create_time, create_date, update_time, update_date)"
    assert expected_columns in columns_section, f"INSERT column list must include `extra`; got {columns_section!r}"
