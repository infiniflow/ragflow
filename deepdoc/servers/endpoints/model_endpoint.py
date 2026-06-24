"""Model metadata endpoint."""

import litserve as ls


class ModelEndpoint(ls.LitAPI):
    """GET /model — returns OSS model metadata."""

    def __init__(self):
        super().__init__()
        self.api_path = "/model"

    def setup(self, device):
        pass

    def decode_request(self, request):
        return None

    def predict(self, _):
        return None

    def encode_response(self, _):
        return {"model": "oss", "version": "1.0"}
