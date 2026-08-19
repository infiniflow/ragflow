#!/usr/bin/env python3
"""Unified OSS DeepDoc Model Server.

Serves DLA, OCR, and TSR models via FastAPI using OSS ONNX Runtime models.

Endpoints:
    POST /predict/dla    — Document Layout Analysis
    POST /predict/ocr    — OCR (detect via ?operator=det, recognize via ?operator=rec)
    POST /predict/tsr    — Table Structure Recognition
    GET  /health         — Health check
    GET  /model          — Model metadata
"""

import argparse
import io
import logging
import os
from contextlib import asynccontextmanager

from fastapi import FastAPI, File, Form, UploadFile
from fastapi.responses import JSONResponse
import uvicorn

from deepdoc.server.adapters.dla_adapter import DLAAdapter
from deepdoc.server.adapters.ocr_adapter import OCRAdapter
from deepdoc.server.adapters.tsr_adapter import TSRAdapter

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
)
logger = logging.getLogger(__name__)

# Global adapters
dla_adapter: DLAAdapter | None = None
ocr_adapter: OCRAdapter | None = None
tsr_adapter: TSRAdapter | None = None


def parse_args():
    parser = argparse.ArgumentParser(
        description="Unified OSS DeepDoc Model Server",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument("--port", type=int, default=9390, help="Serving port (default: 9390)")
    parser.add_argument("--timeout", type=int, default=100, help="Request timeout in seconds (default: 100)")
    parser.add_argument(
        "--model-dir",
        type=str,
        default=os.path.join(os.path.dirname(__file__), "..", "..", "..", "rag", "res", "deepdoc"),
        help="Model file directory",
    )
    parser.add_argument("--disable-dla", action="store_true", dest="disable_dla", default=False, help="Disable DLA endpoint")
    parser.add_argument("--disable-ocr", action="store_true", dest="disable_ocr", default=False, help="Disable OCR endpoint")
    parser.add_argument("--disable-tsr", action="store_true", dest="disable_tsr", default=False, help="Disable TSR endpoint")
    parser.add_argument("--log-level", type=str, default="INFO", help="Logging level")
    return parser.parse_args()


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Initialize models on startup."""
    global dla_adapter, ocr_adapter, tsr_adapter

    args = parse_args()
    model_dir = os.path.abspath(args.model_dir)
    logger.info("Model directory: %s", model_dir)

    if not args.disable_dla:
        dla_adapter = DLAAdapter(model_dir=model_dir)
        dla_adapter.load()
        logger.info("DLA endpoint enabled")

    if not args.disable_ocr:
        ocr_adapter = OCRAdapter(model_dir=model_dir)
        ocr_adapter.load()
        logger.info("OCR endpoint enabled")

    if not args.disable_tsr:
        tsr_adapter = TSRAdapter(model_dir=model_dir)
        tsr_adapter.load()
        logger.info("TSR endpoint enabled")

    yield

    # Cleanup
    if ocr_adapter:
        ocr_adapter.close()


app = FastAPI(title="DeepDoc OSS Server", lifespan=lifespan)


@app.get("/health")
async def health():
    return {"status": "ok"}


@app.get("/model")
async def model_info():
    return {"model": "oss", "version": "1.0"}


@app.post("/predict/dla")
async def predict_dla(file: UploadFile = File(...)):
    if dla_adapter is None:
        return JSONResponse(status_code=503, content={"error": "DLA endpoint disabled"})

    data = await file.read()
    result = dla_adapter(data)
    return {"bboxes": result}


@app.post("/predict/ocr")
async def predict_ocr(
    operator: str = Form(...),
    file: UploadFile = File(...),
):
    if ocr_adapter is None:
        return JSONResponse(status_code=503, content={"error": "OCR endpoint disabled"})

    operator = operator.strip().lower()
    if operator not in ("det", "rec"):
        return JSONResponse(
            status_code=400,
            content={"error": f"Invalid operator '{operator}' (must be 'det' or 'rec')"},
        )

    data = await file.read()
    if operator == "det":
        result = ocr_adapter.detect(data)
    else:
        result = ocr_adapter.recognize(data)
    return result


@app.post("/predict/tsr")
async def predict_tsr(file: UploadFile = File(...)):
    if tsr_adapter is None:
        return JSONResponse(status_code=503, content={"error": "TSR endpoint disabled"})

    data = await file.read()
    result = tsr_adapter(data)
    return {"bboxes": result}


def main():
    args = parse_args()
    logging.getLogger().setLevel(getattr(logging, args.log_level.upper(), "INFO"))

    logger.info("Starting server on port %d...", args.port)
    uvicorn.run(app, host="0.0.0.0", port=args.port, timeout_keep_alive=args.timeout)


if __name__ == "__main__":
    main()
