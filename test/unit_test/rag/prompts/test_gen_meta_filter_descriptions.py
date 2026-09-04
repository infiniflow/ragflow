"""gen_meta_filter must be able to say what a metadata key means.

A value space made of codes -- "SP", "DRP", "PSV" -- tells the model which
values exist but not what any of them stands for, so a question phrased in
words cannot be mapped onto one. RAGFlow already stores a description per key
in the dataset's metadata config; these tests cover threading it into the
prompt.
"""

import json
from unittest.mock import AsyncMock, MagicMock

import pytest

pytestmark = pytest.mark.p2

from rag.prompts.generator import gen_meta_filter

VALUE_SPACE = {"phase": ["SP", "DRP"], "doc_type": ["report"]}


def _chat_mdl():
    mdl = MagicMock()
    mdl.max_length = 131072
    mdl.async_chat = AsyncMock(return_value=json.dumps({"logic": "and", "conditions": []}))
    return mdl


async def _prompt_for(descriptions):
    mdl = _chat_mdl()
    await gen_meta_filter(mdl, VALUE_SPACE, "q", descriptions=descriptions)
    return mdl.async_chat.await_args[0][0]


@pytest.mark.asyncio
async def test_descriptions_reach_the_prompt():
    prompt = await _prompt_for({"phase": "SP = building permit documentation"})
    assert "What the keys mean" in prompt
    assert "SP = building permit documentation" in prompt


@pytest.mark.asyncio
async def test_no_descriptions_leaves_the_prompt_unchanged():
    assert "What the keys mean" not in await _prompt_for(None)
    assert "What the keys mean" not in await _prompt_for({})


@pytest.mark.asyncio
async def test_descriptions_for_keys_that_are_not_offered_are_dropped():
    """semi_auto narrows the keys; a description for an excluded key would
    invite a condition on something the caller deliberately withheld."""
    prompt = await _prompt_for({"phase": "phase code", "revision": "drawing revision"})
    assert "phase code" in prompt
    assert "drawing revision" not in prompt


@pytest.mark.asyncio
async def test_empty_descriptions_are_dropped():
    prompt = await _prompt_for({"phase": "", "doc_type": None})
    assert "What the keys mean" not in prompt


@pytest.mark.asyncio
async def test_non_ascii_descriptions_stay_readable():
    """json.dumps escapes non-ASCII by default, which would hand the model
    \\u0161 instead of the word it has to match against the question."""
    prompt = await _prompt_for({"phase": "SP = dokumentácia pre stavebné povolenie"})
    assert "dokumentácia pre stavebné povolenie" in prompt
