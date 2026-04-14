"""
OpenDataLoader PDF parsing microservice.

Accepts a PDF via multipart POST /file_parse and returns the parsed
JSON document and Markdown text produced by opendataloader_pdf.convert().
The JRE requirement is encapsulated inside this container; the RAGFlow
host does not need Java installed.
"""

from __future__ import annotations

import json
import logging
import tempfile
from pathlib import Path
from typing import Optional

from fastapi import FastAPI, File, Form, UploadFile
from fastapi.responses import JSONResponse

try:
    import opendataloader_pdf
except Exception as _exc:
    opendataloader_pdf = None
    logging.warning(f"opendataloader_pdf not importable: {_exc}")

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("opendataloader-service")

app = FastAPI(title="OpenDataLoader PDF Service", version="1.0.0")


@app.get("/health")
def health():
    if opendataloader_pdf is None:
        return JSONResponse(
            status_code=503,
            content={"status": "unavailable", "reason": "opendataloader_pdf not installed"},
        )
    return {"status": "ok"}


@app.post("/file_parse")
async def file_parse(
    file: UploadFile = File(...),
    hybrid: Optional[str] = Form(None),
    image_output: Optional[str] = Form(None),
    sanitize: Optional[str] = Form(None),
):
    """Parse a PDF file using opendataloader_pdf and return JSON + Markdown."""
    if opendataloader_pdf is None:
        return JSONResponse(
            status_code=503,
            content={"error": "opendataloader_pdf not installed in this container"},
        )

    # Convert sanitize string to bool if provided
    sanitize_val: Optional[bool] = None
    if sanitize is not None:
        sanitize_lower = sanitize.strip().lower()
        if sanitize_lower in ("1", "true", "yes", "on"):
            sanitize_val = True
        elif sanitize_lower in ("0", "false", "no", "off"):
            sanitize_val = False

    pdf_bytes = await file.read()
    filename = file.filename or "input.pdf"

    with tempfile.TemporaryDirectory() as workdir:
        workdir_path = Path(workdir)
        input_path = workdir_path / filename
        input_path.write_bytes(pdf_bytes)

        out_dir = workdir_path / "out"
        out_dir.mkdir()

        convert_kwargs: dict = {
            "input_path": [str(input_path)],
            "output_dir": str(out_dir),
            "format": "markdown,json",
        }
        if hybrid:
            convert_kwargs["hybrid"] = hybrid
        if image_output:
            convert_kwargs["image_output"] = image_output
        if sanitize_val is not None:
            convert_kwargs["sanitize"] = sanitize_val

        try:
            logger.info(f"Converting '{filename}' kwargs={list(convert_kwargs.keys())}")
            opendataloader_pdf.convert(**convert_kwargs)
        except Exception as exc:
            logger.exception(f"convert() failed for '{filename}'")
            return JSONResponse(status_code=500, content={"error": str(exc)})

        json_doc = None
        md_text = None

        for p in sorted(out_dir.rglob("*.json")):
            try:
                with open(p, encoding="utf-8") as fh:
                    json_doc = json.load(fh)
                break
            except Exception:
                continue

        for p in sorted(out_dir.rglob("*.md")):
            try:
                md_text = p.read_text(encoding="utf-8")
                break
            except Exception:
                continue

        logger.info(f"Done '{filename}': json={'yes' if json_doc else 'no'}, md={'yes' if md_text else 'no'}")
        return {"json_doc": json_doc, "md_text": md_text}
