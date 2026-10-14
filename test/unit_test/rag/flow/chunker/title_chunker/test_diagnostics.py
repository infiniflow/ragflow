from types import SimpleNamespace

import pytest

from rag.flow.chunker.title_chunker.group_chunker import GroupTitleChunker
from rag.flow.chunker.title_chunker.hierarchy_chunker import HierarchyTitleChunker


class FakeProcess:
    def __init__(self, method="hierarchy", preview=True):
        self._id = "title"
        self._canvas = SimpleNamespace(is_preview_target=lambda component_id: preview and component_id == self._id)
        self._param = SimpleNamespace(
            method=method,
            levels=[[r"^#\s+", r"^##\s+"]],
            hierarchy=2,
            include_heading_content=True,
            root_chunk_as_heading=False,
        )
        self.outputs = {}

    def set_output(self, key, value):
        self.outputs[key] = value

    def output(self, key=None):
        if key is None:
            return self.outputs
        return self.outputs.get(key)

    def callback(self, *_args):
        return None


def upstream(text):
    return SimpleNamespace(
        output_format="text",
        text_result=text,
        markdown_result=None,
        html_result=None,
        json_result=None,
        chunks=None,
        file={},
    )


@pytest.mark.asyncio
async def test_hierarchy_preview_uses_actual_tree_paths_and_chunk_records():
    process = FakeProcess(method="hierarchy")
    chunker = HierarchyTitleChunker(process, upstream("# A\nbody A\n## B\nbody B"))

    await chunker.invoke()

    debug = process._title_chunker_debug
    assert debug["mode"] == "hierarchy"
    assert [heading["level"] for heading in debug["headings"]] == [1, 2]
    assert debug["headings"][0]["match"]["pattern"] == r"^#\s+"
    assert debug["hierarchy"][0]["children"][0]["title_record_indexes"] == [2]
    assert debug["chunks"]
    assert debug["chunks"][0]["record_indexes"]
    assert debug["chunks"][0]["content"] == process.output("chunks")[0]["text"]


@pytest.mark.asyncio
async def test_group_preview_reports_boundaries_without_hierarchy():
    process = FakeProcess(method="group")
    chunker = GroupTitleChunker(process, upstream("# A\nbody A\n## B\nbody B"))

    await chunker.invoke()

    debug = process._title_chunker_debug
    assert debug["mode"] == "group"
    assert "hierarchy" not in debug
    assert debug["section_ids"]
    assert debug["chunks"]
    assert debug["chunks"][0]["record_indexes"]
    assert debug["chunks"][0]["content"] == process.output("chunks")[0]["text"]


@pytest.mark.asyncio
async def test_preview_disabled_preserves_normal_output_schema():
    process = FakeProcess(method="hierarchy", preview=False)
    chunker = HierarchyTitleChunker(process, upstream("# A\nbody A"))

    await chunker.invoke()

    assert "_title_chunker_debug" not in process.outputs
    assert list(process.outputs) == ["output_format", "chunks"]
    assert process.output("chunks")[0]["text"] == "# A\nbody A\n"


@pytest.mark.asyncio
async def test_group_section_ids_align_after_root_chunk_is_trimmed(monkeypatch):
    monkeypatch.setattr(
        "rag.flow.chunker.title_chunker.group_chunker.num_tokens_from_string",
        lambda text: len(text.split()),
    )
    body_a = " ".join(["body"] * 40)
    document = f"# A\n{body_a}\n## B\nbody B"

    baseline_process = FakeProcess(method="group")
    baseline_chunker = GroupTitleChunker(baseline_process, upstream(document))
    await baseline_chunker.invoke()
    assert len(baseline_process.output("chunks")) == 2

    process = FakeProcess(method="group")
    process._param.root_chunk_as_heading = True
    chunker = GroupTitleChunker(process, upstream(document))

    await chunker.invoke()

    debug = process._title_chunker_debug
    assert debug["section_ids"] == [1]
    assert len(process.output("chunks")) == len(baseline_process.output("chunks")) - 1
    assert len(debug["chunks"]) == len(process.output("chunks"))
    assert len(debug["chunks"]) == 1
    assert debug["chunks"][0]["content"] == process.output("chunks")[0]["text"]


def test_repeated_record_reference_keeps_explicit_source_indexes():
    process = FakeProcess(method="hierarchy")
    chunker = HierarchyTitleChunker(process, upstream(""))
    record = {"text": "# A", "doc_type_kwd": "text"}

    chunker.build_chunks(
        [record, record],
        {
            "levels": [1, 1],
            "most_level": 1,
            "source": "frequency",
            "matches": [None, None],
        },
    )

    assert chunker._preview_record_indexes == [[0], [1]]
