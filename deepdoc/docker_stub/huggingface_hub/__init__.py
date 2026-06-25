# Stub huggingface_hub for Docker — models should be available locally.
# If this is called, something went wrong with model path resolution.
def snapshot_download(**kwargs):
    raise RuntimeError(
        f"huggingface_hub unavailable in Docker. "
        f"snapshot_download called with: {kwargs}. "
        f"Check that models exist at /app/rag/res/deepdoc/"
    )
