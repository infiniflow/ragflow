#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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

"""
Tests for RaptorService.

Coverage is driven through the public entry point `run_raptor_for_kb()`.

Design principles:
- All orchestration behavior is validated through the public API.
- Only stable pure helpers (`_collect_doc_info`, `_schedule_raptor_cleanup`)
  are tested directly.
- Internal methods (`_run_file_level_raptor`, `_run_dataset_level_raptor`,
  `_should_skip_raptor`, `_load_doc_chunks`, `_load_all_doc_chunks`,
  `_generate_raptor`, `_get_raptor_chunk_methods`) are NOT tested directly —
  their behavior is covered by exercising `run_raptor_for_kb()` with
  appropriate mocks.
"""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from rag.svr.task_executor_refactor.raptor_service import RaptorService
from test.unit_test.rag.svr.task_executor_refactor.conftest import make_task_context


# =============================================================================
# Stable Pure Helpers (tested directly)
# =============================================================================


class TestRaptorServiceInit:
    """Tests for RaptorService initialization."""

    def test_init_stores_task_context(self, mock_raptor_context):
        svc = RaptorService(mock_raptor_context)
        assert svc._task_context is mock_raptor_context

    def test_init_uses_provided_kb_id(self, mock_raptor_context):
        mock_raptor_context.kb_id = "custom_kb"
        svc = RaptorService(mock_raptor_context)
        assert svc._task_context.kb_id == "custom_kb"


class TestRaptorServiceCollectDocInfo:
    """Tests for _collect_doc_info — stable pure data aggregation (classmethod)."""

    def _make_mock_doc(self, name, type, parser_id, parser_config):
        """Create a mock document with accessible attributes."""
        mock_doc = MagicMock()
        mock_doc.name = name
        mock_doc.type = type
        mock_doc.parser_id = parser_id
        mock_doc.parser_config = parser_config
        return mock_doc

    def test_collect_doc_info_success(self):
        doc_ids = ["doc_1", "doc_2"]

        mock_doc_1 = self._make_mock_doc(name="", type="pdf", parser_id="naive", parser_config={})
        mock_doc_2 = self._make_mock_doc(name="doc2.txt", type="txt", parser_id="manual", parser_config={"chunk_token_num": 512})

        def get_by_id_side_effect(doc_id):
            if doc_id == "doc_1":
                return True, mock_doc_1
            if doc_id == "doc_2":
                return True, mock_doc_2
            return False, None

        with patch("rag.svr.task_executor_refactor.raptor_service.DocumentService") as mock_ds:
            mock_ds.get_by_id = MagicMock(side_effect=get_by_id_side_effect)
            result = RaptorService._collect_doc_info(doc_ids)

        assert len(result) == 2
        assert result["doc_1"]["name"] == ""
        assert result["doc_1"]["type"] == "pdf"
        assert result["doc_1"]["parser_id"] == "naive"
        assert result["doc_2"]["name"] == "doc2.txt"
        assert result["doc_2"]["type"] == "txt"
        assert result["doc_2"]["parser_id"] == "manual"
        assert result["doc_2"]["parser_config"] == {"chunk_token_num": 512}

    def test_collect_doc_info_empty_input(self):
        result = RaptorService._collect_doc_info([])
        assert result == {}

    def test_collect_doc_info_deduplicates_doc_ids(self):
        """Duplicate doc_ids should be deduplicated."""
        doc_ids = ["doc_1", "doc_1", "doc_2"]

        mock_doc = self._make_mock_doc(name="test.pdf", type="pdf", parser_id="naive", parser_config={})

        called_ids = []

        def get_by_id_side_effect(doc_id):
            called_ids.append(doc_id)
            return True, mock_doc

        with patch("rag.svr.task_executor_refactor.raptor_service.DocumentService") as mock_ds:
            mock_ds.get_by_id = MagicMock(side_effect=get_by_id_side_effect)
            result = RaptorService._collect_doc_info(doc_ids)

        assert sorted(called_ids) == ["doc_1", "doc_2"]
        assert len(result) == 2

    def test_collect_doc_info_missing_document(self):
        doc_ids = ["doc_1", "missing_doc"]

        mock_doc = self._make_mock_doc(name="test.pdf", type="pdf", parser_id="naive", parser_config={})

        def get_by_id_side_effect(doc_id):
            if doc_id == "doc_1":
                return True, mock_doc
            return False, None

        with patch("rag.svr.task_executor_refactor.raptor_service.DocumentService") as mock_ds:
            mock_ds.get_by_id = MagicMock(side_effect=get_by_id_side_effect)
            result = RaptorService._collect_doc_info(doc_ids)

        assert len(result) == 1
        assert "doc_1" in result
        assert "missing_doc" not in result


class TestRaptorServiceScheduleRaptorCleanup:
    """Tests for _schedule_raptor_cleanup — stable pure data operation (classmethod)."""

    def test_schedule_cleanup_adds_entry(self):
        cleanup_list = []
        RaptorService._schedule_raptor_cleanup("doc_1", "tree_builder_a", cleanup_list)
        assert cleanup_list == [("doc_1", "tree_builder_a")]

    def test_schedule_cleanup_deduplicates(self):
        cleanup_list = [("doc_1", "tree_builder_a")]
        RaptorService._schedule_raptor_cleanup("doc_1", "tree_builder_a", cleanup_list)
        assert len(cleanup_list) == 1

    def test_schedule_cleanup_keep_method_none(self):
        cleanup_list = []
        RaptorService._schedule_raptor_cleanup("doc_1", None, cleanup_list)
        assert cleanup_list == [("doc_1", None)]

    def test_schedule_cleanup_multiple_docs(self):
        cleanup_list = []
        RaptorService._schedule_raptor_cleanup("doc_1", "t1", cleanup_list)
        RaptorService._schedule_raptor_cleanup("doc_2", "t2", cleanup_list)
        RaptorService._schedule_raptor_cleanup("doc_3", None, cleanup_list)
        assert len(cleanup_list) == 3
        assert ("doc_1", "t1") in cleanup_list
        assert ("doc_2", "t2") in cleanup_list
        assert ("doc_3", None) in cleanup_list


# =============================================================================
# Public Entry Point Tests
# =============================================================================


class TestRaptorServiceRunRaptorForKb:
    """Tests for run_raptor_for_kb() — the public entry point.

    All orchestration behavior (file-level vs dataset-level dispatch,
    chunk loading, skip logic, cleanup scheduling) is validated through
    this method by mocking internal helpers and observing:
    - Return values (chunks, token_count, cleanup_raptor_chunks)
    - Mock call patterns (which internal method was invoked, with what args)
    """

    @pytest.fixture
    def sample_chunks(self):
        """Sample RAPTOR summary chunks returned by internal methods."""
        return [{"id": "chunk_1", "content_with_weight": "Summary 1"}]

    @pytest.fixture
    def raptor_config_file_scope(self):
        """RAPTOR config with file-level scope."""
        return {
            "raptor": {
                "tree_builder": "raptor",
                "clustering_method": "gmm",
                "scope": "file",
                "prompt": "summarize",
                "max_token": 512,
                "threshold": 0.5,
                "max_cluster": 64,
                "random_seed": 42,
            }
        }

    @pytest.fixture
    def raptor_config_dataset_scope(self):
        """RAPTOR config with dataset-level scope."""
        return {
            "raptor": {
                "tree_builder": "raptor",
                "clustering_method": "gmm",
                "scope": "dataset",
                "prompt": "summarize",
                "max_token": 512,
                "threshold": 0.5,
                "max_cluster": 64,
                "random_seed": 42,
            }
        }

    # ---- Basic dispatch (file-level scope) ----

    @pytest.mark.asyncio
    async def test_run_raptor_for_kb_file_scope_delegates_to_file_level(self, mock_raptor_context, sample_chunks, raptor_config_file_scope):
        """When scope='file', _run_file_level_raptor is called."""
        svc = RaptorService(mock_raptor_context)
        doc_ids = ["doc_1", "doc_2"]
        chat_mdl = MagicMock()
        embd_mdl = MagicMock()
        vector_size = 128

        with (
            patch.object(
                svc,
                "_collect_doc_info",
                return_value={
                    "doc_1": {"name": "a.pdf", "type": "pdf", "parser_id": "naive", "parser_config": {}},
                    "doc_2": {"name": "b.pdf", "type": "pdf", "parser_id": "naive", "parser_config": {}},
                },
            ),
            patch.object(svc, "_run_file_level_raptor", new_callable=AsyncMock) as mock_file,
            patch.object(svc, "_run_dataset_level_raptor", new_callable=AsyncMock) as mock_dataset,
        ):
            mock_file.return_value = (sample_chunks, 42)
            chunks, tk_count, cleanup = await svc.run_raptor_for_kb(raptor_config_file_scope, chat_mdl, embd_mdl, vector_size, doc_ids)

        mock_file.assert_called_once()
        mock_dataset.assert_not_called()
        assert chunks == sample_chunks
        assert tk_count == 42

    # ---- Basic dispatch (dataset-level scope) ----

    @pytest.mark.asyncio
    async def test_run_raptor_for_kb_dataset_scope_delegates_to_dataset_level(self, mock_raptor_context, sample_chunks, raptor_config_dataset_scope):
        """When scope='dataset', _run_dataset_level_raptor is called."""
        svc = RaptorService(mock_raptor_context)
        doc_ids = ["doc_1"]
        chat_mdl = MagicMock()
        embd_mdl = MagicMock()
        vector_size = 128

        with (
            patch.object(
                svc,
                "_collect_doc_info",
                return_value={
                    "doc_1": {"name": "a.pdf", "type": "pdf", "parser_id": "naive", "parser_config": {}},
                },
            ),
            patch.object(svc, "_run_file_level_raptor", new_callable=AsyncMock) as mock_file,
            patch.object(svc, "_run_dataset_level_raptor", new_callable=AsyncMock) as mock_dataset,
        ):
            mock_dataset.return_value = (sample_chunks, 99)
            chunks, tk_count, cleanup = await svc.run_raptor_for_kb(raptor_config_dataset_scope, chat_mdl, embd_mdl, vector_size, doc_ids)

        mock_dataset.assert_called_once()
        mock_file.assert_not_called()
        assert chunks == sample_chunks
        assert tk_count == 99

    # ---- Empty / no documents ----

    @pytest.mark.asyncio
    async def test_run_raptor_for_kb_empty_doc_ids(self, mock_raptor_context, raptor_config_file_scope):
        """Empty doc_ids returns empty results."""
        svc = RaptorService(mock_raptor_context)
        chat_mdl = MagicMock()
        embd_mdl = MagicMock()

        with (
            patch.object(svc, "_collect_doc_info", return_value={}),
            patch.object(svc, "_run_file_level_raptor", new_callable=AsyncMock) as mock_file,
            patch.object(svc, "_run_dataset_level_raptor", new_callable=AsyncMock),
        ):
            mock_file.return_value = ([], 0)
            chunks, tk_count, cleanup = await svc.run_raptor_for_kb(raptor_config_file_scope, chat_mdl, embd_mdl, 128, [])

        assert chunks == []
        assert tk_count == 0
        assert cleanup == []

    # ---- Cleanup scheduling through the public API ----

    @pytest.mark.asyncio
    async def test_run_raptor_for_kb_returns_cleanup_list(self, mock_raptor_context, raptor_config_file_scope):
        """Cleanup list from internal method is propagated to caller."""
        svc = RaptorService(mock_raptor_context)
        doc_ids = ["doc_1"]
        chat_mdl = MagicMock()
        embd_mdl = MagicMock()

        expected_cleanup = [("doc_1", "tree_builder_a")]

        with (
            patch.object(
                svc,
                "_collect_doc_info",
                return_value={
                    "doc_1": {"name": "a.pdf", "type": "pdf", "parser_id": "naive", "parser_config": {}},
                },
            ),
            patch.object(svc, "_run_file_level_raptor", new_callable=AsyncMock) as mock_file,
        ):

            async def mock_run_file(*args, **kwargs):
                cleanup_list = args[11]
                cleanup_list.append(("doc_1", "tree_builder_a"))
                return [{"id": "c1"}], 10

            mock_file.side_effect = mock_run_file
            chunks, tk_count, cleanup = await svc.run_raptor_for_kb(raptor_config_file_scope, chat_mdl, embd_mdl, 128, doc_ids)

        assert cleanup == expected_cleanup

    # ---- Dispatch with missing raptor config key ----

    @pytest.mark.asyncio
    async def test_run_raptor_for_kb_defaults_to_file_scope_when_no_raptor_key(self, mock_raptor_context):
        """When kb_parser_config has no 'raptor' key, defaults to file scope."""
        svc = RaptorService(mock_raptor_context)
        doc_ids = ["doc_1"]
        chat_mdl = MagicMock()
        embd_mdl = MagicMock()
        config = {}  # No raptor key at all

        with (
            patch.object(
                svc,
                "_collect_doc_info",
                return_value={
                    "doc_1": {"name": "a.pdf", "type": "pdf", "parser_id": "naive", "parser_config": {}},
                },
            ),
            patch.object(svc, "_run_file_level_raptor", new_callable=AsyncMock) as mock_file,
            patch.object(svc, "_run_dataset_level_raptor", new_callable=AsyncMock) as mock_dataset,
        ):
            mock_file.return_value = ([], 0)
            await svc.run_raptor_for_kb(config, chat_mdl, embd_mdl, 128, doc_ids)

        mock_file.assert_called_once()
        mock_dataset.assert_not_called()

    # ---- Vector dimension name construction ----

    @pytest.mark.asyncio
    async def test_run_raptor_for_kb_passes_vector_size_to_file_level(self, mock_raptor_context, sample_chunks, raptor_config_file_scope):
        """Vector size is used to construct vctr_nm and passed to internal method."""
        svc = RaptorService(mock_raptor_context)
        doc_ids = ["doc_1"]
        chat_mdl = MagicMock()
        embd_mdl = MagicMock()
        vector_size = 256

        with (
            patch.object(
                svc,
                "_collect_doc_info",
                return_value={
                    "doc_1": {"name": "a.pdf", "type": "pdf", "parser_id": "naive", "parser_config": {}},
                },
            ),
            patch.object(svc, "_run_file_level_raptor", new_callable=AsyncMock) as mock_file,
        ):
            mock_file.return_value = (sample_chunks, 10)
            await svc.run_raptor_for_kb(raptor_config_file_scope, chat_mdl, embd_mdl, vector_size, doc_ids)

        # Verify _run_file_level_raptor received vctr_nm with the correct vector size
        # Positional args: 0=raptor_config, 1=tree_builder, 2=clustering_method,
        #   3=chat_mdl, 4=embd_mdl, 5=vctr_nm
        positional_args = mock_file.call_args[0]
        assert positional_args[5] == "q_256_vec"

    # ---- Document info collection through public API ----

    @pytest.mark.asyncio
    async def test_run_raptor_for_kb_collects_doc_info(self, mock_raptor_context, raptor_config_file_scope):
        """Document info is collected before dispatching to internal methods."""
        svc = RaptorService(mock_raptor_context)
        doc_ids = ["doc_a"]
        chat_mdl = MagicMock()
        embd_mdl = MagicMock()

        expected_info = {"doc_a": {"name": "file.pdf", "type": "pdf", "parser_id": "naive", "parser_config": {}}}

        with patch.object(svc, "_collect_doc_info", return_value=expected_info) as mock_collect, patch.object(svc, "_run_file_level_raptor", new_callable=AsyncMock) as mock_file:
            mock_file.return_value = ([], 0)
            await svc.run_raptor_for_kb(raptor_config_file_scope, chat_mdl, embd_mdl, 128, doc_ids)

        mock_collect.assert_called_once_with(doc_ids)
        # Verify doc_info_by_id was passed as positional arg[7] to _run_file_level_raptor
        positional_args = mock_file.call_args[0]
        assert positional_args[7] == expected_info


class TestRaptorServiceFileLevelRaptorCheckpoint:
    """Tests for _run_file_level_raptor checkpoint behavior.

    Verifies the fix that moves progress_cb and continue to the outer if
    block so progress is reported even when existing_methods == {tree_builder}.
    """

    @pytest.mark.asyncio
    async def test_file_level_raptor_existing_methods_exact_match_updates_progress(self):
        """When existing_methods == {tree_builder}, progress_cb is still called."""
        ctx = make_task_context()
        svc = RaptorService(ctx)

        doc_ids = ["doc_1"]
        doc_info_by_id = {"doc_1": {"name": "a.pdf", "type": "pdf", "parser_id": "naive", "parser_config": {}}}
        raptor_config = {
            "scope": "file",
            "max_cluster": 64,
            "prompt": "test prompt",
            "max_token": 256,
            "threshold": 0.1,
            "random_seed": 0,
            "clustering_method": "gmm",
            "tree_builder": "raptor",
            "ext": {},
        }

        with patch.object(svc, "_get_raptor_chunk_methods", new_callable=AsyncMock) as mock_methods, patch.object(svc, "_should_skip_raptor", return_value=False):
            mock_methods.return_value = {"raptor"}

            result = await svc._run_file_level_raptor(
                raptor_config=raptor_config,
                tree_builder="raptor",
                clustering_method="gmm",
                chat_mdl=MagicMock(),
                embd_mdl=MagicMock(),
                vctr_nm="q_128_vec",
                doc_ids=doc_ids,
                doc_info_by_id=doc_info_by_id,
                max_errors=3,
                res=[],
                tk_count=0,
                cleanup_raptor_chunks=[],
            )

            msg_calls = [call.kwargs.get("msg", "") for call in ctx.progress_cb.call_args_list if call.kwargs.get("msg") is not None]
            assert any("already has" in m for m in msg_calls), f"Expected 'already has' progress message, got: {msg_calls}"

            prog_calls = [call.kwargs.get("prog") for call in ctx.progress_cb.call_args_list if call.kwargs.get("prog") is not None]
            assert len(prog_calls) > 0, "Expected progress_cb to be called with prog update"
            assert result[0] == []
