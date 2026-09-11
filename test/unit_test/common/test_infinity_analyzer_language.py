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
"""Unit tests for per-dataset analyzer selection on the Infinity engine.

Infinity analyzes a fulltext query with the analyzer of the index it matches
against -- there is no per-query analyzer to pass -- so a dataset language that
changes tokenization has to be baked into the index when the table is created.
Chunk tables are per-dataset (``{index_name}_{dataset_id}``), which is what
makes that possible.

Run with: python -m pytest test/unit_test/common/test_infinity_analyzer_language.py -v
"""

from unittest.mock import MagicMock, patch

import pytest

pytestmark = pytest.mark.p2

# ``common.doc_store.infinity_conn_base`` is reached through ``common.settings``,
# which imports the rag- and memory-side connectors; pre-loading it resolves the
# partial-module circular import before we touch the base module.
import common.settings  # noqa: F401,E402
from common.doc_store import infinity_conn_base  # noqa: E402
from common.doc_store.infinity_conn_base import _analyzer_for_language as analyzer_for_language  # noqa: E402


class TestAnalyzerForLanguage:
    @pytest.mark.parametrize(
        ("analyzer", "language", "expected"),
        [
            ("rag-coarse", "Slovak", "rag-coarse-slovak"),
            ("rag-fine", "Slovak", "rag-fine-slovak"),
            ("rag-coarse", "czech", "rag-coarse-czech"),
            ("rag", "Slovak", "rag-slovak"),
            ("rag-fine", "  CZECH  ", "rag-fine-czech"),
        ],
    )
    def test_folding_languages_get_their_own_analyzer(self, analyzer, language, expected):
        assert analyzer_for_language(analyzer, language) == expected

    @pytest.mark.parametrize("language", ["English", "Dutch", "Chinese", "", None])
    def test_other_languages_keep_the_default_analyzer(self, language):
        """Their datasets are already indexed under the default analyzer.

        Every language outside the folding set only selects a Snowball stemmer,
        so suffixing it here would change tokenization for existing datasets
        without reindexing them.
        """
        assert analyzer_for_language("rag-coarse", language) == "rag-coarse"

    @pytest.mark.parametrize("analyzer", ["whitespace-#", "rankfeatures", "ragged"])
    def test_non_rag_analyzers_are_untouched(self, analyzer):
        assert analyzer_for_language(analyzer, "Slovak") == analyzer


class TestFulltextIndexName:
    def test_language_is_visible_in_the_index_name(self):
        assert infinity_conn_base._fulltext_index_name("content", "rag-coarse-slovak") == "ft_content_rag_coarse_slovak"

    def test_coarse_still_sorts_before_fine(self):
        """Infinity picks a field's first index by name when the query names none.

        The coarse analyzer has to stay that first index once the language
        suffix is appended, or a query would be analyzed fine-grained.
        """
        coarse = infinity_conn_base._fulltext_index_name("content", analyzer_for_language("rag-coarse", "slovak"))
        fine = infinity_conn_base._fulltext_index_name("content", analyzer_for_language("rag-fine", "slovak"))
        assert coarse < fine


class TestHasForeignRagIndex:
    """A table's rag analyzer is settled by its first fulltext index."""

    def _wanted(self, language):
        return infinity_conn_base._wanted_fulltext_indexes("content", {"analyzer": ["rag-coarse", "rag-fine"]}, language)

    def test_a_default_index_is_foreign_to_a_slovak_dataset(self):
        """The case that silently costs a Slovak dataset its folding.

        The table was created without a language, so adding the slovak indexes
        beside the defaults would achieve nothing: the unsuffixed name sorts
        first and keeps winning.
        """
        existing = ["q_vec_idx", "ft_content_rag_coarse", "ft_content_rag_fine"]
        assert infinity_conn_base._has_foreign_rag_index(existing, "content", self._wanted("Slovak")) is True

    def test_a_slovak_index_is_foreign_to_a_default_dataset(self):
        existing = ["ft_content_rag_coarse_slovak", "ft_content_rag_fine_slovak"]
        assert infinity_conn_base._has_foreign_rag_index(existing, "content", self._wanted(None)) is True

    def test_matching_indexes_are_not_foreign(self):
        existing = ["ft_content_rag_coarse_slovak", "ft_content_rag_fine_slovak"]
        assert infinity_conn_base._has_foreign_rag_index(existing, "content", self._wanted("slovak")) is False

    def test_a_missing_variant_is_still_repairable(self):
        """Half-built tables must not be frozen: only other languages are."""
        existing = ["ft_content_rag_coarse_slovak"]
        wanted = self._wanted("slovak")
        assert infinity_conn_base._has_foreign_rag_index(existing, "content", wanted) is False
        assert "ft_content_rag_fine_slovak" in wanted

    def test_another_field_is_not_consulted(self):
        assert infinity_conn_base._has_foreign_rag_index(["ft_docnm_rag_coarse"], "content", self._wanted("slovak")) is False

    def test_a_shorter_field_name_is_not_a_prefix_match(self):
        """``name`` must not be satisfied by ``name_kwd``'s index."""
        wanted = infinity_conn_base._wanted_fulltext_indexes("name", {"analyzer": ["rag-coarse"]}, "slovak")
        assert infinity_conn_base._has_foreign_rag_index(["ft_name_kwd_rag_coarse"], "name", wanted) is False


class _FakeTable:
    def __init__(self, existing=()):
        self.indexes = []
        self.existing = list(existing)

    def create_index(self, name, index_info, conflict_type=None):
        self.indexes.append((name, index_info))
        return MagicMock(error_code=0)

    def list_indexes(self):
        return MagicMock(index_names=self.existing)


def _created_fulltext_analyzers(language, existing=()):
    """Run ``create_idx`` against a fake Infinity and collect its analyzers."""
    table = _FakeTable(existing)
    db = MagicMock()
    db.create_table.return_value = table
    conn = MagicMock()
    conn.create_database.return_value = db

    connection = MagicMock()
    connection.connPool.get_conn.return_value = conn
    connection.dbName = "default_db"
    connection.mapping_file_name = "infinity_mapping.json"
    connection.logger = MagicMock()

    infinity_conn_base.InfinityConnectionBase.create_idx(
        connection,
        "ragflow_tenant",
        "kb1",
        1024,
        None,
        language,
    )

    return {name: info.params.get("ANALYZER") for name, info in table.indexes if name.startswith("ft_")}


class TestCreateIdxAnalyzers:
    """The real ``conf/infinity_mapping.json`` drives these, not a fixture."""

    def test_slovak_dataset_indexes_with_the_slovak_analyzer(self):
        analyzers = _created_fulltext_analyzers("Slovak")
        assert analyzers["ft_content_rag_coarse_slovak"] == "rag-coarse-slovak"
        assert analyzers["ft_content_rag_fine_slovak"] == "rag-fine-slovak"
        # No unsuffixed rag index may exist next to them: Infinity would analyze
        # the query with whichever of a field's indexes sorts first.
        assert "ft_content_rag_coarse" not in analyzers

    def test_keyword_fields_are_unaffected_by_the_language(self):
        analyzers = _created_fulltext_analyzers("Slovak")
        assert analyzers["ft_tag_kwd_whitespace__"] == "whitespace-#"
        assert analyzers["ft_tag_feas_rankfeatures"] == "rankfeatures"

    def test_an_existing_default_table_is_left_as_it_is(self):
        """No suffixed index is added beside a default one.

        Infinity would keep analyzing queries with the unsuffixed index, so the
        extra indexes would cost storage and build time and change nothing.
        """
        existing = ["q_vec_idx"] + [f"ft_{f}_rag_{g}" for f in ("content", "docnm", "questions", "important_keywords", "authors") for g in ("coarse", "fine")]
        analyzers = _created_fulltext_analyzers("Slovak", existing=existing)
        assert not [name for name in analyzers if name.endswith("_slovak")]

    def test_default_language_keeps_todays_analyzers(self):
        analyzers = _created_fulltext_analyzers(None)
        assert analyzers["ft_content_rag_coarse"] == "rag-coarse"
        assert analyzers["ft_content_rag_fine"] == "rag-fine"
        assert not [name for name in analyzers if name.endswith("_slovak") or name.endswith("_czech")]


class TestMigrateDbPreservesTheDatasetAnalyzer:
    """``_migrate_db`` walks every table with no idea of any dataset's language.

    It must therefore leave a field's existing rag index alone instead of adding
    the default one beside it, which would win on name order and undo the
    dataset's language.
    """

    def _run_migrate(self, existing_indexes):
        table = _FakeTable(existing_indexes)
        table.show_columns = MagicMock(return_value={"name": ["content", "tag_kwd", "tag_feas"]})
        table.add_columns = MagicMock(return_value=MagicMock(error_code=0))

        db = MagicMock()
        db.list_tables.return_value = MagicMock(table_names=["ragflow_tenant_kb1"])
        db.get_table.return_value = table
        conn = MagicMock()
        conn.create_database.return_value = db

        connection = MagicMock()
        connection.dbName = "default_db"
        connection.mapping_file_name = "infinity_mapping.json"
        connection.table_name_prefix = "ragflow_"
        connection.logger = MagicMock()

        with patch.object(infinity_conn_base.infinity, "ErrorCode", MagicMock(OK=0)):
            infinity_conn_base.InfinityConnectionBase._migrate_db(connection, conn)
        return [name for name, _ in table.indexes]

    def test_a_slovak_table_keeps_only_its_slovak_indexes(self):
        created = self._run_migrate(["q_vec_idx", "ft_content_rag_coarse_slovak", "ft_content_rag_fine_slovak"])
        assert "ft_content_rag_coarse" not in created
        assert "ft_content_rag_fine" not in created

    def test_a_table_with_no_rag_index_yet_still_gets_one(self):
        created = self._run_migrate(["q_vec_idx"])
        assert "ft_content_rag_coarse" in created
        assert "ft_content_rag_fine" in created
