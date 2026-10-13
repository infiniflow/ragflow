import pytest

from rag.flow.pipeline import Pipeline


class _FakeParam:
    def update(self, _params):
        return None

    def check(self):
        return None


class _FakeComponent:
    fail_ids = set()

    def __init__(self, canvas, component_id, _param):
        self.canvas = canvas
        self._id = component_id
        self.component_name = component_id
        self._output = {"component_id": component_id}
        self._error = ""
        self.invocations = 0

    async def invoke(self, **inputs):
        self.invocations += 1
        self._output = {"component_id": self._id, "inputs": inputs}
        if self._id in self.fail_ids:
            self._error = "component failed"

    def error(self):
        return self._error

    def output(self, key=None):
        if key is None:
            return self._output
        return self._output.get(key)

    def get_downstream(self):
        return self.canvas.components[self._id]["downstream"]


def _patch_fake_components(monkeypatch):
    def fake_component_class(name):
        if name.endswith("Param"):
            return _FakeParam
        return _FakeComponent

    monkeypatch.setattr("agent.canvas.component_class", fake_component_class)


def _dsl():
    return {
        "components": {
            "file": {"obj": {"component_name": "File", "params": {}}, "downstream": ["title"]},
            "title": {"obj": {"component_name": "TitleChunker", "params": {}}, "downstream": ["index"]},
            "index": {"obj": {"component_name": "Indexer", "params": {}}, "downstream": []},
        },
        "path": [],
        "graph": {"nodes": []},
    }


@pytest.mark.asyncio
async def test_pipeline_stops_after_target_and_returns_target_output(monkeypatch):
    _patch_fake_components(monkeypatch)
    events = []
    monkeypatch.setattr(
        Pipeline,
        "callback",
        lambda self, component_name, progress=None, message="": events.append(
            (component_name, progress, message)
        ),
    )

    pipeline = Pipeline(_dsl(), stop_after_component_id="title")
    output = await pipeline.run()

    assert output["component_id"] == "title"
    assert pipeline.components["file"]["obj"].invocations == 1
    assert pipeline.components["title"]["obj"].invocations == 1
    assert pipeline.components["index"]["obj"].invocations == 0
    assert events[-1][0] == "END"
    assert sum(event[0] == "END" for event in events) == 1
    assert '"component_id": "title"' in events[-1][2]


def test_pipeline_rejects_unknown_target(monkeypatch):
    _patch_fake_components(monkeypatch)

    with pytest.raises(ValueError, match="Unknown stop target component"):
        Pipeline(_dsl(), stop_after_component_id="missing")


@pytest.mark.asyncio
async def test_pipeline_target_failure_does_not_expand_downstream(monkeypatch):
    _patch_fake_components(monkeypatch)
    _FakeComponent.fail_ids = {"title"}
    events = []
    monkeypatch.setattr(
        Pipeline,
        "callback",
        lambda self, component_name, progress=None, message="": events.append(
            (component_name, progress, message)
        ),
    )

    try:
        pipeline = Pipeline(_dsl(), stop_after_component_id="title")
        output = await pipeline.run()
    finally:
        _FakeComponent.fail_ids = set()

    assert output == {}
    assert pipeline.components["index"]["obj"].invocations == 0
    assert events[-1][0] == "END"
    assert events[-1][1] == -1


@pytest.mark.asyncio
async def test_pipeline_without_target_preserves_full_execution(monkeypatch):
    _patch_fake_components(monkeypatch)
    monkeypatch.setattr(Pipeline, "callback", lambda *args, **kwargs: None)

    pipeline = Pipeline(_dsl())
    output = await pipeline.run()

    assert output["component_id"] == "index"
    assert pipeline.components["file"]["obj"].invocations == 1
    assert pipeline.components["title"]["obj"].invocations == 1
    assert pipeline.components["index"]["obj"].invocations == 1