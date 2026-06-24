#!/usr/bin/env python3
"""Unified OSS DeepDoc Model Server.

Serves DLA, OCR, and TSR models via LiteServe using OSS ONNX Runtime models.

Endpoints:
    POST /predict/dla    — Document Layout Analysis
    POST /predict/ocr    — OCR (detect via ?operator=det, recognize via ?operator=rec)
    POST /predict/tsr    — Table Structure Recognition
    GET  /health         — Health check
"""

import argparse
import logging
import os

import litserve as ls

from deepdoc.servers.endpoints.dla_endpoint import DLAEndpoint
from deepdoc.servers.endpoints.ocr_endpoint import OCREndpoint
from deepdoc.servers.endpoints.tsr_endpoint import TSREndpoint

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
)
logger = logging.getLogger(__name__)


def parse_args():
    parser = argparse.ArgumentParser(
        description="Unified OSS DeepDoc Model Server",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "--port", type=int, default=8124, help="Serving port (default: 8124)"
    )
    parser.add_argument(
        "--timeout", type=int, default=100, help="Request timeout in seconds (default: 100)"
    )
    parser.add_argument(
        "--model-dir",
        type=str,
        default=os.path.join(
            os.path.dirname(__file__), "..", "..", "..", "rag", "res", "deepdoc"
        ),
        help="Model file directory",
    )
    parser.add_argument(
        "--disable-dla", action="store_true", dest="disable_dla", default=False,
        help="Disable DLA endpoint"
    )
    parser.add_argument(
        "--disable-ocr", action="store_true", dest="disable_ocr", default=False,
        help="Disable OCR endpoint"
    )
    parser.add_argument(
        "--disable-tsr", action="store_true", dest="disable_tsr", default=False,
        help="Disable TSR endpoint"
    )
    parser.add_argument(
        "--gpu", action="store_true", default=False,
        help="Enable GPU inference (uses CUDAExecutionProvider for ONNX Runtime)"
    )
    parser.add_argument(
        "--workers", type=int, default=1,
        help="Workers per device (default: 1; GPU mode may use more)"
    )
    parser.add_argument("--log-level", type=str, default="INFO", help="Logging level")
    return parser.parse_args()


def main():
    args = parse_args()
    logging.getLogger().setLevel(getattr(logging, args.log_level.upper(), "INFO"))

    model_dir = os.path.abspath(args.model_dir)
    logger.info("Model directory: %s", model_dir)

    apis = []
    if not args.disable_dla:
        apis.append(DLAEndpoint(model_dir=model_dir))
        logger.info("DLA endpoint enabled")
    if not args.disable_ocr:
        apis.append(OCREndpoint(model_dir=model_dir))
        logger.info("OCR endpoint enabled")
    if not args.disable_tsr:
        apis.append(TSREndpoint(model_dir=model_dir))
        logger.info("TSR endpoint enabled")

    if not apis:
        logger.error("No endpoints enabled")
        return

    # GPU mode: detect CUDA availability, fall back to CPU if unavailable
    accelerator = "cpu"
    workers = 1
    if args.gpu:
        try:
            import torch
            if torch.cuda.is_available() and torch.cuda.device_count() > 0:
                accelerator = "cuda"
                workers = args.workers
                os.environ.setdefault("PARALLEL_DEVICES", str(workers))
                logger.info("GPU mode: accelerator=cuda workers=%d (CUDA: %d devices)",
                            workers, torch.cuda.device_count())
            else:
                logger.warning("--gpu set but no CUDA device found, falling back to CPU")
        except ImportError:
            logger.warning("--gpu set but torch import failed, falling back to CPU")
    else:
        logger.info("CPU mode: accelerator=cpu workers=1")

    server = ls.LitServer(
        lit_api=apis,
        accelerator=accelerator,
        workers_per_device=workers,
        timeout=args.timeout,
        restart_workers=True,
    )

    # /model — returns OSS model metadata (no LitServe path conflict)
    @server.app.get("/model")
    async def model_info():
        return {"model": "oss", "version": "1.0"}

    logger.info("Starting server on port %d...", args.port)
    server.run(port=args.port)


if __name__ == "__main__":
    main()
