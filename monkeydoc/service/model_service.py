from __future__ import annotations

"""MonkeyOCR runtime service (Phase 1 stub).

This module exposes a thin service wrapper intended to host the MonkeyOCR
runtime in future phases. In Phase 1 it provides a minimal, no-op API to
validate imports and lifecycle without invoking any ML models.
"""

from dataclasses import dataclass
from typing import Optional, List, Dict, Any

from pathlib import Path
from PIL import Image as PILImage
import tempfile
import os

# Light-weight import for category mapping and OMR helpers
try:
    from monkeyocr.magic_pdf.config.ocr_content_type import CategoryId  # type: ignore
    from monkeyocr.magic_pdf.model.custom_model import MonkeyOCR  # type: ignore
    from monkeyocr.cedd_parse import (  # type: ignore
        classify_image as cedd_classify_image,
        extract_rating_answers_from_cropped_image as cedd_extract_ratings,
    )
except Exception:  # pragma: no cover - optional at import time
    CategoryId = None
    MonkeyOCR = None
    cedd_classify_image = None
    cedd_extract_ratings = None


@dataclass
class _ServiceState:
    """Internal state holder for the MonkeyOCR service.

    Phase 1 stores only the configuration path. Later phases will cache
    model handles and GPU resources here.
    """

    config_path: Optional[str] = None


class MonkeyOCRService:
    """Lazy, process-wide MonkeyOCR runtime service (Phase 1).

    Responsibilities in Phase 1:
    - Accept a config path and expose a no-op lifecycle.
    - Provide placeholders for detection/OCR APIs so downstream code can
      compile and unit tests can exercise the call chain.
    """

    _state: _ServiceState | None = None

    @classmethod
    def load(cls, config_path: Optional[str]) -> None:
        """Initialize the singleton service.

        Parameters
        ----------
        config_path: Optional[str]
            Path to a configuration file. Stored for later use.
        """

        cls._state = _ServiceState(config_path=config_path)

    @classmethod
    def cleanup(cls) -> None:
        """Release resources and reset the singleton state.

        Phase 1 simply clears the in-memory state. Later phases will
        close model handles and free GPU memory.
        """

        cls._state = None

    # Phase 2 — Layout detection
    @classmethod
    def detect_layout(
        cls,
        page_images: List[PILImage.Image],
        zoomin: int = 3,
        config_path: Optional[str] = None,
        batch_size: int = 8,
        model=None,
    ) -> List[List[Dict[str, Any]]]:
        """Run DocLayout on page images and normalize to DeepDoc-like blocks.

        Parameters
        ----------
        page_images: list[PIL.Image]
            Rendered page rasters at 72 * zoomin DPI.
        zoomin: int
            Scale factor used at rasterization time.
        config_path: Optional[str]
            Path to MonkeyOCR model config. Uses default if None.
        batch_size: int
            Batch size for layout model.

        Returns
        -------
        list[list[dict]]
            Per-page lists of blocks with fields: x0, x1, top, bottom, type.
        """

        # Short-circuit if no images
        if not page_images:
            return [[]]

        # Resolve config path
        if config_path is None:
            # project_root/monkeyocr/model_configs.yaml
            project_root = Path(__file__).resolve().parents[2]
            default_cfg = project_root / "monkeyocr" / "model_configs.yaml"
            config_path = str(default_cfg)

        # Prepare output container
        per_page_blocks: List[List[Dict[str, Any]]] = [[] for _ in range(len(page_images))]

        # Fallback if MonkeyOCR is not importable
        if MonkeyOCR is None:
            return per_page_blocks

        owns_model = False
        try:
            if model is None:
                model = MonkeyOCR(config_path)
                owns_model = True

            # MonkeyOCR expects PIL.Image list
            layout_results_all = model.layout_model.batch_predict(page_images, batch_size)

            # Map MonkeyOCR category ids to DeepDoc taxonomy
            def normalize_label(cid: int) -> str:
                if CategoryId is None:
                    return "text"
                mapping = {
                    CategoryId.Text: "text",
                    CategoryId.Title: "title",
                    CategoryId.ImageBody: "figure",
                    CategoryId.ImageCaption: "figure caption",
                    CategoryId.TableBody: "table",
                    CategoryId.TableCaption: "table caption",
                    CategoryId.TableFootnote: "reference",
                    CategoryId.ImageFootnote: "reference",
                    CategoryId.InterlineEquation_Layout: "equation",
                    CategoryId.InterlineEquation_YOLO: "equation",
                    CategoryId.OcrText: "text",
                }
                return mapping.get(cid, "text")

            def poly_to_bbox(poly: List[float]) -> tuple[float, float, float, float]:
                xs = poly[0::2]
                ys = poly[1::2]
                return min(xs), min(ys), max(xs), max(ys)

            for page_index, detections in enumerate(layout_results_all):
                blocks: List[Dict[str, Any]] = []
                for det in detections:
                    cid = int(det.get("category_id", -1))
                    poly = det.get("poly") or []
                    if not poly or cid == getattr(CategoryId, "Abandon", 2):
                        continue
                    x0_px, y0_px, x1_px, y1_px = poly_to_bbox(poly)
                    # Return pixel coordinates; parser will rescale to PDF units using page size
                    blocks.append(
                        {
                            "x0": float(x0_px),
                            "x1": float(x1_px),
                            "top": float(y0_px),
                            "bottom": float(y1_px),
                            "type": normalize_label(cid),
                            "unit": "px",
                        }
                    )
                per_page_blocks[page_index] = blocks

        except Exception:
            # Swallow errors and return empty layout; later phases can fallback
            return per_page_blocks
        finally:
            try:
                if owns_model and model is not None:
                    model.cleanup()
            except Exception:
                pass

        return per_page_blocks

    # Phase 4 — Reading order using LayoutReader
    @classmethod
    def order_lines(
        cls,
        line_boxes: List[tuple[float, float, float, float]],
        page_w: float,
        page_h: float,
        model=None,
    ) -> List[int]:
        """Order line boxes using MonkeyOCR LayoutReader when available.

        Returns a permutation of indices representing the reading order.
        Fallback: simple sort by (top, left) if model is unavailable.
        """

        if not line_boxes:
            return []

        try:
            if MonkeyOCR is None and model is None:
                raise RuntimeError("MonkeyOCR not available")

            owns_model = False
            if model is None:
                # Resolve default config
                project_root = Path(__file__).resolve().parents[2]
                default_cfg = project_root / "monkeyocr" / "model_configs.yaml"
                model = MonkeyOCR(str(default_cfg))
                owns_model = True

            # Import helpers lazily; fall back if unavailable
            try:
                from monkeyocr.magic_pdf.model.sub_modules.reading_oreder.layoutreader.helpers import (  # type: ignore
                    boxes2inputs, parse_logits, prepare_inputs,
                )
            except Exception:
                # Fallback: naive order
                return sorted(range(len(line_boxes)), key=lambda i: (line_boxes[i][1], line_boxes[i][0]))

            # Scale to 1000x1000 as in MonkeyOCR pipeline
            x_scale = 1000.0 / max(page_w, 1e-6)
            y_scale = 1000.0 / max(page_h, 1e-6)
            boxes_scaled = []
            for left, top, right, bottom in line_boxes:
                left = max(0, min(1000, round(left * x_scale)))
                right = max(0, min(1000, round(right * x_scale)))
                top = max(0, min(1000, round(top * y_scale)))
                bottom = max(0, min(1000, round(bottom * y_scale)))
                boxes_scaled.append([left, top, right, bottom])

            # Prepare inputs and run model
            inputs = boxes2inputs(boxes_scaled)
            inputs = prepare_inputs(inputs, model.layoutreader_model)
            import torch  # local import to avoid hard dep at module load
            with torch.no_grad():
                logits = model.layoutreader_model(**inputs).logits.cpu().squeeze(0)
            orders = parse_logits(logits, len(boxes_scaled))
            return orders
        except Exception:
            # Fallback: simple visual order
            return sorted(range(len(line_boxes)), key=lambda i: (line_boxes[i][1], line_boxes[i][0]))
        finally:
            try:
                if 'owns_model' in locals() and owns_model and model is not None:
                    model.cleanup()
            except Exception:
                pass

    # Phase 3 — OCR on crops
    @classmethod
    def ocr_text(
        cls,
        images: List[PILImage.Image],
        instruction: Optional[str] = None,
        config_path: Optional[str] = None,
        batch_size: int = 16,
        model=None,
    ) -> List[str]:
        """Recognize text for a batch of cropped regions using MonkeyOCR's VLM.

        Parameters
        ----------
        images: list[PIL.Image]
            Cropped images corresponding to text-like regions.
        instruction: Optional[str]
            Prompt guiding recognition, defaults to a generic extraction prompt.
        config_path: Optional[str]
            Path to MonkeyOCR model config. Uses default if None.
        batch_size: int
            Batch size hint; the underlying API handles batching.

        Returns
        -------
        list[str]
            Recognized texts, one per input image (empty string on failure).
        """

        if not images:
            return []

        # Resolve config path
        if config_path is None:
            project_root = Path(__file__).resolve().parents[2]
            default_cfg = project_root / "monkeyocr" / "model_configs.yaml"
            config_path = str(default_cfg)

        prompt = instruction or "Please output the text content from the image."

        if MonkeyOCR is None:
            return [""] * len(images)

        owns_model = False
        try:
            if model is None:
                model = MonkeyOCR(config_path)
                owns_model = True
            outputs: List[str] = []
            effective_bs = max(1, int(batch_size))
            for i in range(0, len(images), effective_bs):
                chunk_imgs = images[i:i + effective_bs]
                questions = [prompt] * len(chunk_imgs)
                chunk_out = model.chat_model.batch_inference(chunk_imgs, questions)
                outputs.extend([o if isinstance(o, str) else str(o) for o in chunk_out])
            return outputs
        except Exception:
            return [""] * len(images)
        finally:
            try:
                if owns_model and model is not None:
                    model.cleanup()
            except Exception:
                pass

    # Phase 5 — OMR (circle forms) batch extraction
    @classmethod
    def omr_ratings_batch(cls, images: List[PILImage.Image]) -> List[Optional[List[int]]]:
        """Extract 5-point rating answers for each image if it's a circle form.

        Returns list of ratings (list[int]) per image, or None if not OMR.
        Uses temporary files to interop with the existing OMR module.
        """

        if not images or cedd_classify_image is None or cedd_extract_ratings is None:
            return [None] * len(images)

        results: List[Optional[List[int]]] = []
        temp_paths: List[str] = []
        try:
            for img in images:
                # Persist to temp JPEG for OMR module
                fd, path = tempfile.mkstemp(suffix=".jpg", prefix="omr_")
                os.close(fd)
                temp_paths.append(path)
                try:
                    img.save(path, format="JPEG")
                except Exception:
                    results.append(None)
                    continue

            for path in temp_paths:
                try:
                    clsf = cedd_classify_image(path)
                    if clsf != "multiple_choice":
                        results.append(None)
                        continue
                    ratings = cedd_extract_ratings(path)
                    # ratings can be None or list of ints
                    if isinstance(ratings, list):
                        results.append(ratings)
                    else:
                        results.append(None)
                except Exception:
                    results.append(None)
        finally:
            for p in temp_paths:
                try:
                    if os.path.exists(p):
                        os.remove(p)
                except Exception:
                    pass

        return results


