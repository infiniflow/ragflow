import asyncio

from rag.advanced_rag.knowlege_compile.wiki_incremental import _wiki_run_refine_tasks


def test_wiki_refine_progress_is_batched_and_reports_completion():
    async def exercise():
        messages = []

        async def complete():
            return None

        await _wiki_run_refine_tasks([complete() for _ in range(391)], messages.append)

        assert len(messages) == 20
        assert messages[0] == "20/391 pages completed."
        assert messages[-1] == "391/391 pages completed."

    asyncio.run(exercise())
