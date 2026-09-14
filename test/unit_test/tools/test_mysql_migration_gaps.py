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
"""Unit tests for ``tools/scripts/mysql_migration.py`` helper functions.

The two static helpers on ``TenantModelInstanceStage`` close the two
gaps in the MySQL data migration that left ``tenant_model_instance``
rows missing the ``api_base`` URL and clashing on the hard-coded
``instance_name='default'``. They are tested here as pure functions
so no database connection is required.
"""

import hashlib
import json

import pytest

from tools.scripts.mysql_migration import TenantModelInstanceStage

_build_instance_extra = TenantModelInstanceStage._build_instance_extra
_assign_instance_names = TenantModelInstanceStage._assign_instance_names


def _dedup(records):
    """Wrap the bound ``_dedup_api_key_records`` instance method so the
    static-style tests can call it without a full instance (no DB).
    """
    instance = TenantModelInstanceStage.__new__(TenantModelInstanceStage)
    # _dedup_api_key_records only touches self._strip_is_tools_from_api_key.
    # Bind the underlying function (not the bound method, which double-passes self).
    instance._strip_is_tools_from_api_key = TenantModelInstanceStage._strip_is_tools_from_api_key
    return instance._dedup_api_key_records(records)


# ---------------------------------------------------------------------------
# _build_instance_extra
# ---------------------------------------------------------------------------


class TestBuildInstanceExtra:
    """Verify ``_build_instance_extra`` produces the right JSON for the
    ``extra`` column on ``tenant_model_instance``.
    """

    def test_none_returns_empty_object(self):
        assert _build_instance_extra(None) == "{}"

    def test_empty_string_returns_empty_object(self):
        assert _build_instance_extra("") == "{}"

    def test_whitespace_only_returns_empty_object(self):
        assert _build_instance_extra("   ") == "{}"
        assert _build_instance_extra("\t\n") == "{}"

    def test_url_returns_base_url_entry(self):
        result = _build_instance_extra("https://api.example.com/v1")
        assert json.loads(result) == {"base_url": "https://api.example.com/v1"}

    def test_url_with_trailing_whitespace_is_accepted(self):
        # ``.strip()`` only checks the truthiness of the result, so a
        # URL with leading/trailing whitespace is still treated as set.
        result = _build_instance_extra("  https://api.example.com  ")
        assert json.loads(result) == {"base_url": "  https://api.example.com  "}

    def test_non_string_returns_empty_object(self):
        # The runtime schema treats ``api_base`` as TEXT; defensive
        # guard returns "{}" for any non-string so the migration does
        # not crash on bad rows.
        assert _build_instance_extra(0) == "{}"
        assert _build_instance_extra(False) == "{}"
        assert _build_instance_extra([]) == "{}"
        assert _build_instance_extra({"base_url": "x"}) == "{}"

    def test_non_ascii_characters_preserved(self):
        # ``ensure_ascii=False`` keeps non-ASCII chars readable in MySQL
        # utf8mb4 rather than expanding them to \uXXXX escapes.
        result = _build_instance_extra("https://例え.test/path")
        assert result == '{"base_url": "https://例え.test/path"}'

    def test_result_is_valid_json(self):
        for value in (None, "", "https://x.test", "  "):
            json.loads(_build_instance_extra(value))


# ---------------------------------------------------------------------------
# _assign_instance_names
# ---------------------------------------------------------------------------


def _rec(tenant_id, llm_factory, api_key, status="1", provider_id="p1", api_base=None):
    """Build a 6-tuple record in the shape the migration SELECT produces
    after the column was added (``api_base`` appended at the end).
    """
    return (tenant_id, llm_factory, api_key, status, provider_id, api_base)


class TestAssignInstanceNames:
    """Verify ``_assign_instance_names`` derives a unique, stable
    ``instance_name`` per (tenant_id, llm_factory, provider_id) group.
    """

    def test_empty_input_returns_empty_list(self):
        assert _assign_instance_names([]) == []

    def test_single_record_gets_default(self):
        records = [_rec("t1", "OpenAI", "sk-aaa")]
        out = _assign_instance_names(records)
        assert len(out) == 1
        assert out[0][-1] == "default"

    def test_two_keys_in_one_group(self):
        records = [
            _rec("t1", "OpenAI", "sk-aaa"),
            _rec("t1", "OpenAI", "sk-bbb"),
        ]
        out = _assign_instance_names(records)
        assert [r[-1] for r in out] == ["default", "default-08f976f4"]
        # hash is sha256("sk-bbb")[:8]; pin the exact value so any
        # accidental change to the digest algorithm is caught.
        expected_hash = hashlib.sha256(b"sk-bbb").hexdigest()[:8]
        assert out[1][-1] == f"default-{expected_hash}"

    def test_three_keys_in_one_group(self):
        records = [
            _rec("t1", "OpenAI", "sk-aaa"),
            _rec("t1", "OpenAI", "sk-bbb"),
            _rec("t1", "OpenAI", "sk-ccc"),
        ]
        out = _assign_instance_names(records)
        # First record always "default"; subsequent records are
        # independent sha256 hashes of their api_key.
        assert out[0][-1] == "default"
        assert out[1][-1] == f"default-{hashlib.sha256(b'sk-bbb').hexdigest()[:8]}"
        assert out[2][-1] == f"default-{hashlib.sha256(b'sk-ccc').hexdigest()[:8]}"
        # All three names must be distinct.
        assert len({r[-1] for r in out}) == 3

    def test_multiple_groups_each_get_a_default(self):
        # Two independent (tenant, factory, provider) groups: the
        # first record of each group must be "default" — not just the
        # first record of the entire input list.
        records = [
            _rec("t1", "OpenAI", "sk-aaa", provider_id="p1"),
            _rec("t1", "OpenAI", "sk-bbb", provider_id="p1"),
            _rec("t2", "OpenAI", "sk-ccc", provider_id="p2"),
            _rec("t2", "OpenAI", "sk-ddd", provider_id="p2"),
        ]
        out = _assign_instance_names(records)
        names = [r[-1] for r in out]
        assert names.count("default") == 2
        assert "default" in names[0] and "default" in names[2]

    def test_grouping_is_by_tenant_factory_provider(self):
        # Same (tenant, factory, provider) → grouped together; same
        # api_key but different provider → different group.
        records = [
            _rec("t1", "OpenAI", "sk-shared", provider_id="p1"),
            _rec("t1", "OpenAI", "sk-shared", provider_id="p1"),
            _rec("t1", "OpenAI", "sk-shared", provider_id="p2"),
        ]
        out = _assign_instance_names(records)
        # p1 has 2 records → 1 default + 1 hashed; p2 has 1 → default.
        p1 = [r for r in out if r[4] == "p1"]
        p2 = [r for r in out if r[4] == "p2"]
        assert p1[0][-1] == "default"
        assert p1[1][-1].startswith("default-")
        assert p2[0][-1] == "default"

    def test_is_idempotent(self):
        records = [
            _rec("t1", "OpenAI", "sk-aaa"),
            _rec("t1", "OpenAI", "sk-bbb"),
            _rec("t1", "OpenAI", "sk-ccc"),
        ]
        first = _assign_instance_names(records)
        second = _assign_instance_names(records)
        assert first == second

    def test_hash_is_stable_across_runs(self):
        # Run a small hash twice and confirm the digest matches a
        # pre-computed reference (catches any accidental switch to
        # hashlib.md5, hashlib.sha1, etc.).
        records = [_rec("t1", "OpenAI", "sk-aaa"), _rec("t1", "OpenAI", "sk-fixed")]
        out = _assign_instance_names(records)
        assert out[1][-1] == "default-23cdf6aa"
        assert out[1][-1] == f"default-{hashlib.sha256(b'sk-fixed').hexdigest()[:8]}"

    def test_empty_api_key_still_gets_unique_name(self):
        # Defensive: if api_key is None or empty (data quality issue),
        # the hash of the empty string is still a valid unique suffix.
        records = [
            _rec("t1", "OpenAI", ""),
            _rec("t1", "OpenAI", ""),
        ]
        out = _assign_instance_names(records)
        assert out[0][-1] == "default"
        assert out[1][-1] == f"default-{hashlib.sha256(b'').hexdigest()[:8]}"

    def test_none_api_key_still_gets_unique_name(self):
        records = [
            _rec("t1", "OpenAI", None),
            _rec("t1", "OpenAI", None),
        ]
        out = _assign_instance_names(records)
        assert out[0][-1] == "default"
        assert out[1][-1] == f"default-{hashlib.sha256(b'').hexdigest()[:8]}"

    def test_output_preserves_all_input_fields(self):
        records = [_rec("t1", "OpenAI", "sk-aaa", status="active", provider_id="p9", api_base="https://x.test")]
        out = _assign_instance_names(records)
        assert len(out[0]) == 7
        tenant_id, llm_factory, api_key, status, provider_id, api_base, instance_name = out[0]
        assert tenant_id == "t1"
        assert llm_factory == "OpenAI"
        assert api_key == "sk-aaa"
        assert status == "active"
        assert provider_id == "p9"
        assert api_base == "https://x.test"
        assert instance_name == "default"

    def test_output_preserves_api_base_for_hashed_records(self):
        records = [
            _rec("t1", "OpenAI", "sk-aaa", api_base="https://a.test"),
            _rec("t1", "OpenAI", "sk-bbb", api_base="https://b.test"),
        ]
        out = _assign_instance_names(records)
        assert out[0][5] == "https://a.test"
        assert out[1][5] == "https://b.test"

    def test_input_order_preserved_within_group(self):
        # First record in the group keeps its position in the output so
        # the "default" name lands on the same row it was always
        # going to land on (backward compatibility for callers that
        # depend on row ordering, e.g. for batched inserts).
        records = [
            _rec("t1", "OpenAI", "sk-first"),
            _rec("t1", "OpenAI", "sk-second"),
            _rec("t1", "OpenAI", "sk-third"),
        ]
        out = _assign_instance_names(records)
        assert [r[2] for r in out] == ["sk-first", "sk-second", "sk-third"]
        assert [r[-1] for r in out] == ["default", f"default-{hashlib.sha256(b'sk-second').hexdigest()[:8]}", f"default-{hashlib.sha256(b'sk-third').hexdigest()[:8]}"]


# ---------------------------------------------------------------------------
# Combined: helper pair matches what the INSERT loop emits
# ---------------------------------------------------------------------------


class TestInsertRowShape:
    """Sanity check that the migration's INSERT loop and the helpers
    agree on the row shape: 6 SELECT fields + instance_name + extra.
    """

    def test_one_record_row_has_seven_fields(self):
        record = _rec("t1", "OpenAI", "sk-aaa", api_base="https://x.test")
        out = _assign_instance_names([record])[0]
        assert len(out) == 7  # 6 SELECT fields + instance_name
        extra = _build_instance_extra(out[5])  # api_base is the 6th field
        assert json.loads(extra) == {"base_url": "https://x.test"}

    def test_two_records_both_get_distinct_names_and_extras(self):
        records = [
            _rec("t1", "OpenAI", "sk-aaa", api_base="https://a.test"),
            _rec("t1", "OpenAI", "sk-bbb", api_base="https://b.test"),
        ]
        out = _assign_instance_names(records)
        names = [r[-1] for r in out]
        extras = [_build_instance_extra(r[5]) for r in out]
        assert names[0] == "default"
        assert names[1] != "default"
        assert json.loads(extras[0]) == {"base_url": "https://a.test"}
        assert json.loads(extras[1]) == {"base_url": "https://b.test"}


# ---------------------------------------------------------------------------
# _dedup_api_key_records
# ---------------------------------------------------------------------------


class TestDedupApiKeyRecords:
    """The migration SELECT returns 6 columns
    (tenant_id, llm_factory, api_key, status, provider_id, api_base) but the
    original dedup unpacked only 5 — raising ``ValueError: too many values to
    unpack`` on every call. These tests pin the 6-tuple shape.
    """

    def test_six_tuple_does_not_raise(self):
        records = [
            _rec("t1", "OpenAI", "sk-aaa", api_base="https://a.test"),
        ]
        # Should not raise ValueError
        result = _dedup(records)
        assert result == records

    def test_keeps_all_rows_when_no_dupes(self):
        records = [
            _rec("t1", "OpenAI", "sk-aaa", api_base="https://a.test"),
            _rec("t1", "OpenAI", "sk-bbb", api_base="https://b.test"),
            _rec("t1", "Anthropic", "sk-ccc", api_base=None),
        ]
        result = _dedup(records)
        assert len(result) == 3

    def test_merges_is_tools_only_differences(self):
        # Two rows that differ only in the ``is_tools`` wrapper around the
        # same logical api_key should collapse to one.
        records = [
            _rec("t1", "OpenAI", '{"api_key": "sk-aaa"}', api_base="https://a.test"),
            _rec("t1", "OpenAI", '{"api_key": "sk-aaa", "is_tools": true}', api_base="https://a.test"),
        ]
        result = _dedup(records)
        assert len(result) == 1
        assert result[0][2] == '{"api_key": "sk-aaa"}'

    def test_preserves_api_base(self):
        records = [
            _rec("t1", "OpenAI", "sk-aaa", api_base="https://a.test"),
            _rec("t1", "OpenAI", "sk-bbb", api_base="https://b.test"),
        ]
        result = _dedup(records)
        # All rows kept (different api_keys), api_base preserved
        assert result[0][5] == "https://a.test"
        assert result[1][5] == "https://b.test"

    def test_does_not_merge_same_api_key_with_different_api_bases(self):
        # Two rows that canonicalize to the same logical API key but
        # carry different api_base values must NOT be merged. Pre-fix,
        # the dedup identity ignored api_base entirely, so the first
        # row could silently shadow the second even though the
        # migration's stated contract is to preserve distinct bases.
        records = [
            _rec("t1", "OpenAI", '{"api_key": "sk-aaa"}', api_base="https://a.test"),
            _rec("t1", "OpenAI", '{"api_key": "sk-aaa", "is_tools": true}', api_base="https://b.test"),
        ]
        result = _dedup(records)
        assert result == records


if __name__ == "__main__":
    raise SystemExit(pytest.main([__file__, "-v"]))
