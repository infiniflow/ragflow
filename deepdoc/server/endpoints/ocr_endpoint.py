"""OCR LitServe endpoint — detect + rec via operator form field."""

import logging

import litserve as ls

from deepdoc.server.adapters.ocr_adapter import OCRAdapter

logger = logging.getLogger(__name__)


class OCREndpoint(ls.LitAPI):
    """OCR endpoint at /predict/ocr.

    Form field 'operator' (det or rec) selects the mode.
    Form field 'request' carries the JPEG image bytes.
    """

    def __init__(self, model_dir: str):
        super().__init__()
        self.api_path = "/predict/ocr"
        self.model_dir = model_dir
        self.adapter: OCRAdapter | None = None

    def setup(self, device):
        self.adapter = OCRAdapter(model_dir=self.model_dir)
        self.adapter.load()
        logger.info("OCR model loaded")

    def decode_request(self, request):
        # Handle both old Starlette UploadFile and new Starlette FormData
        if hasattr(request, "file"):
            data = request.file.read()
            # Try to read operator from the underlying request context
            operator = getattr(self, "_request", None)
            if operator is not None:
                operator = operator.query_params.get("operator", "")
            else:
                operator = ""
        else:
            # FormData: get file and operator form fields
            data = request.get("request").file.read()
            op_val = request.get("operator")
            operator = str(op_val) if op_val else ""

        if not data:
            raise ValueError("Empty request body")
        if len(data) > 50 * 1024 * 1024:
            raise ValueError("Image too large")

        operator = operator.strip().lower()
        if operator not in ("det", "rec"):
            raise ValueError(f"Invalid or missing operator '{operator}' (must be 'det' or 'rec')")

        return operator, data

    def predict(self, inputs: tuple):
        operator, image_data = inputs
        if operator == "det":
            return self.adapter.detect(image_data)
        else:
            return self.adapter.recognize(image_data)

    def encode_response(self, output):
        return output
