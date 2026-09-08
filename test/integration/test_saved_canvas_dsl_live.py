"""Load persisted current and historical Canvas DSL in the local Docker DB.

This is intentionally opt-in because it uses the contour's persisted data. The
synthetic round trip creates and deletes its own records; the existing-record
audits are read-only. No provider or model setting is changed.
"""

import json
import os
from pathlib import Path
from uuid import uuid4

import pytest


def _require_live_contour() -> None:
    if os.environ.get("RAGFLOW_SAVED_DSL_TEST") != "1":
        pytest.skip("Requires the explicitly enabled local Docker saved-DSL contour")


def _load_error(dsl, tenant_id: str, canvas_id: str) -> str | None:
    from agent.canvas import Canvas

    try:
        Canvas(json.dumps(dsl, ensure_ascii=False), tenant_id, canvas_id=canvas_id)
    except LookupError:
        return "missing_runtime_dependency"
    except ValueError:
        return "invalid_required_component_config"
    except Exception as exc:
        return type(exc).__name__
    return None


def _failure_summary(failures: dict[str, str]) -> dict[str, object]:
    classes: dict[str, int] = {}
    for failure_class in failures.values():
        classes[failure_class] = classes.get(failure_class, 0) + 1
    return {
        "count": len(failures),
        "classes": classes,
        "sample_ids": sorted(failures)[:5],
    }


def _minimal_saved_dsl(message: str) -> dict:
    template_path = Path(os.environ.get("RAGFLOW_MRZ_TEMPLATE", "/ragflow/agent/templates/mrz_document_reader.json"))
    dsl = json.loads(template_path.read_text(encoding="utf-8"))["dsl"]
    begin = dsl["components"]["begin"]
    result = dsl["components"]["Message:MRZResult"]
    begin["downstream"] = ["Message:T1SavedDsl"]
    result["upstream"] = ["begin"]
    result["obj"]["params"]["content"] = [message]
    dsl["components"] = {"begin": begin, "Message:T1SavedDsl": result}
    dsl["path"] = []
    dsl["history"] = []
    dsl["retrieval"] = []
    dsl["graph"] = {"nodes": [], "edges": []}
    return dsl


@pytest.mark.p0
def test_synthetic_current_and_historical_saved_dsl_round_trip_and_cleanup():
    _require_live_contour()
    from agent.canvas import Canvas
    from api.db.db_models import UserCanvas, UserCanvasVersion
    from api.db.services.canvas_service import UserCanvasService
    from api.db.services.user_canvas_version import UserCanvasVersionService
    from common import settings

    settings.init_settings()
    owner = UserCanvas.select(UserCanvas.user_id).first()
    assert owner is not None, "The local contour needs one existing tenant owner"
    canvas_id = uuid4().hex
    first = _minimal_saved_dsl("T1 saved DSL first revision")
    second = _minimal_saved_dsl("T1 saved DSL second revision")
    try:
        UserCanvasService.insert(id=canvas_id, user_id=owner.user_id, title="T1 synthetic saved DSL", dsl=first)
        _, first_created = UserCanvasVersionService.save_or_replace_latest(
            user_canvas_id=canvas_id,
            dsl=first,
            title="T1 synthetic released revision",
            release=True,
        )
        assert first_created is True
        UserCanvasService.update_by_id(canvas_id, {"dsl": second})
        _, second_created = UserCanvasVersionService.save_or_replace_latest(
            user_canvas_id=canvas_id,
            dsl=second,
            title="T1 synthetic current revision",
            release=False,
        )
        assert second_created is True

        current = UserCanvas.get_by_id(canvas_id)
        versions = list(UserCanvasVersion.select().where(UserCanvasVersion.user_canvas_id == canvas_id))
        assert len(versions) == 2
        Canvas(json.dumps(current.dsl, ensure_ascii=False), owner.user_id, canvas_id=canvas_id)
        for version in versions:
            Canvas(json.dumps(version.dsl, ensure_ascii=False), owner.user_id, canvas_id=canvas_id)
    finally:
        UserCanvasVersion.delete().where(UserCanvasVersion.user_canvas_id == canvas_id).execute()
        UserCanvas.delete().where(UserCanvas.id == canvas_id).execute()

    assert UserCanvas.get_or_none(UserCanvas.id == canvas_id) is None
    assert UserCanvasVersion.select().where(UserCanvasVersion.user_canvas_id == canvas_id).count() == 0


@pytest.mark.p0
def test_current_saved_canvas_dsl_still_loads():
    _require_live_contour()
    from api.db.db_models import UserCanvas
    from common import settings

    settings.init_settings()
    failures = {canvas.id: error for canvas in UserCanvas.select() if (error := _load_error(canvas.dsl, canvas.user_id, canvas.id)) is not None}
    assert not failures, _failure_summary(failures)


@pytest.mark.p0
def test_attached_historical_canvas_versions_still_load():
    _require_live_contour()
    from api.db.db_models import UserCanvas, UserCanvasVersion
    from common import settings

    settings.init_settings()
    owners = {canvas.id: canvas.user_id for canvas in UserCanvas.select(UserCanvas.id, UserCanvas.user_id)}
    failures = {}
    checked = 0
    for version in UserCanvasVersion.select():
        owner = owners.get(version.user_canvas_id)
        if owner is None:
            continue
        checked += 1
        error = _load_error(version.dsl, owner, version.user_canvas_id)
        if error is not None:
            failures[version.id] = error
    assert checked > 0, "No attached historical Canvas versions were found"
    assert not failures, _failure_summary(failures)
