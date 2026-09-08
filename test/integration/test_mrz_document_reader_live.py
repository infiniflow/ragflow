"""Opt-in real-image execution of the deployed MRZ agent template.

Run inside the local RAGFlow application container with
``RAGFLOW_MRZ_TEST_IMAGE`` pointing to a binary PNG/JPEG fixture.  The test
uses the tenant's existing Image2Text configuration and does not replace the
OCR stage or the deterministic checksum validator with mocks.
"""

import asyncio
import base64
import json
import os
from pathlib import Path
from uuid import uuid4

import pytest


EXPECTED_DOCUMENT_NUMBER = "L898902C3"


@pytest.mark.p0
def test_mrz_template_reads_and_validates_real_image_fixture():
    fixture = os.environ.get("RAGFLOW_MRZ_TEST_IMAGE")
    if not fixture:
        pytest.skip("Requires RAGFLOW_MRZ_TEST_IMAGE in the local Docker contour")

    from agent.canvas import Canvas
    from api.db.db_models import Tenant
    from common import settings

    source = Path(fixture)
    assert source.is_file()
    mime_type = "image/png" if source.suffix.lower() == ".png" else "image/jpeg"
    tenant_id = os.environ.get("RAGFLOW_MRZ_TEST_TENANT", "").strip()
    if tenant_id:
        tenant = Tenant.get_by_id(tenant_id)
        assert tenant.img2txt_id
    else:
        eligible = list(Tenant.select().where(Tenant.img2txt_id.is_null(False), Tenant.img2txt_id != ""))
        assert len(eligible) == 1, "Set RAGFLOW_MRZ_TEST_TENANT when the contour has zero or multiple Image2Text tenants"
        tenant = eligible[0]
        tenant_id = tenant.id

    settings.init_settings()
    template_path = Path(os.environ.get("RAGFLOW_MRZ_TEMPLATE", "/ragflow/agent/templates/mrz_document_reader.json"))
    template = json.loads(template_path.read_text(encoding="utf-8"))
    template["dsl"]["components"]["Agent:MRZOCR"]["obj"]["params"]["llm_id"] = tenant.img2txt_id
    data_uri = f"data:{mime_type};base64,{base64.b64encode(source.read_bytes()).decode('ascii')}"

    async def execute():
        canvas = Canvas(json.dumps(template["dsl"], ensure_ascii=False), tenant_id, task_id="t1-local-mrz-" + uuid4().hex)
        canvas.globals["sys.files"] = [data_uri]
        events = []
        async for event in canvas.run(query="Распознайте MRZ на изображении.", user_id=tenant_id):
            events.append(event)
        return canvas, events

    canvas, events = asyncio.run(execute())
    assert events
    ocr = canvas.get_component_obj("Agent:MRZOCR")
    validator = canvas.get_component_obj("CodeExec:MRZValidate")
    assert not ocr.error()
    assert not validator.error()
    result = json.loads(validator.output("result"))
    assert result.get("valid") is True, {"ocr": ocr.output("content"), "validator": result}
    assert result.get("format") == "TD3"
    assert result["attributes"]["document_number"] == EXPECTED_DOCUMENT_NUMBER
