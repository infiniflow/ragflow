from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from rag.svr.task_executor_refactor.task_handler import TaskHandler


def make_ctx():
    ctx = MagicMock()
    ctx.id = "task-1"
    ctx.doc_id = "doc-1"
    ctx.name = "doc.pdf"
    ctx.from_page = 0
    ctx.to_page = 12
    ctx.parser_config = {}
    ctx.write_interceptor = None
    ctx.has_canceled_func = MagicMock(return_value=False)
    ctx.progress_cb = MagicMock()
    return ctx


@pytest.mark.asyncio
async def test_post_chunking_gate_waits_when_counter_not_zero():
    handler = TaskHandler(ctx=make_ctx())
    handler._run_document_structure_compile = AsyncMock()
    handler._run_raptor = AsyncMock()

    with patch(
        "rag.svr.task_executor_refactor.task_handler.credit_doc_chunking_task",
        return_value=1,
    ), patch(
        "rag.svr.task_executor_refactor.task_handler.clear_doc_chunking_counter",
    ) as clear_counter:
        assert await handler._run_document_post_chunking_if_last(MagicMock(), 128, 0.0, 4, 40)

    handler._run_document_structure_compile.assert_not_called()
    handler._run_raptor.assert_not_called()
    clear_counter.assert_not_called()


@pytest.mark.asyncio
async def test_post_chunking_gate_runs_compile_and_raptor_when_counter_hits_zero():
    ctx = make_ctx()
    ctx.parser_config = {"raptor": {"use_raptor": True}}
    handler = TaskHandler(ctx=ctx)
    handler._run_document_structure_compile = AsyncMock()
    handler._run_raptor = AsyncMock()

    with patch(
        "rag.svr.task_executor_refactor.task_handler.credit_doc_chunking_task",
        return_value=0,
    ), patch(
        "rag.svr.task_executor_refactor.task_handler.DocumentService.get_by_id",
        return_value=(True, MagicMock()),
    ), patch(
        "rag.svr.task_executor_refactor.task_handler.clear_doc_chunking_counter",
    ) as clear_counter:
        assert await handler._run_document_post_chunking_if_last(MagicMock(), 128, 0.0, 4, 40)

    handler._run_document_structure_compile.assert_awaited_once()
    handler._run_raptor.assert_awaited_once()
    assert handler._run_raptor.await_args.kwargs["mark_done"] is False
    clear_counter.assert_called_once_with("doc-1")
