"""DLA LitServe endpoint."""

import logging

import litserve as ls

from deepdoc.server.adapters.dla_adapter import DLAAdapter

logger = logging.getLogger(__name__)


class DLAEndpoint(ls.LitAPI):
    """Document Layout Analysis endpoint at /predict/dla."""

    def __init__(self, model_dir: str, thr: float = 0.2):
        super().__init__()
        self.api_path = "/predict/dla"
        self.model_dir = model_dir
        self.thr = thr
        self.adapter: DLAAdapter | None = None

    def setup(self, device):
        self.adapter = DLAAdapter(model_dir=self.model_dir, thr=self.thr)
        self.adapter.load()
        logger.info("DLA model loaded")

    def decode_request(self, request):
        # Handle both Starlette UploadFile (old) and FormData (Starlette >=1.3)
        if hasattr(request, "file"):
            data = request.file.read()
        else:
            data = request.get("request").file.read()
        if not data:
            raise ValueError("Empty request body")
        if len(data) > 50 * 1024 * 1024:  # 50MB
            raise ValueError("Image too large")
        return data

    def predict(self, image_data: bytes):
        return self.adapter(image_data)

    def encode_response(self, output):
        return {"bboxes": output}
