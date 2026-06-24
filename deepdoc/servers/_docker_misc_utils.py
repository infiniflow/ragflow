# Stub common.misc_utils for Docker deepdoc service.


def pip_install_torch(*args, **kwargs):
    """Attempt to import torch so cuda_is_available() in load_model() works.

    Torch is pre-installed in the GPU Docker image.  If the import fails
    (CPU-only image or torch not installed), cuda_is_available() returns
    False and load_model() falls back to CPUExecutionProvider transparently.
    """
    try:
        import torch  # noqa: F401
    except ImportError:
        pass
