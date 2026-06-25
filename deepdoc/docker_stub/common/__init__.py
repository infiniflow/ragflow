# Stub common.__init__ for Docker deepdoc service.
import os


class _Settings:
    PARALLEL_DEVICES = int(os.environ.get("PARALLEL_DEVICES", "0"))

settings = _Settings()
