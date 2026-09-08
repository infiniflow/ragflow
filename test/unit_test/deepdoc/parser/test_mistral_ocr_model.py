def test_mistral_ocr_model_registers_in_registry():
    from rag.llm import OcrModel

    assert "Mistral OCR" in OcrModel


def test_mistral_ocr_model_reads_flat_env_config():
    from rag.llm.ocr_model import MistralOcrModel
    import json

    key = json.dumps({"MISTRAL_OCR_API_KEY": "sk-x", "MISTRAL_OCR_BASE_URL": "https://api.mistral.ai/v1"})
    mdl = MistralOcrModel(key=key, model_name="mistral-ocr-latest")
    assert mdl.api_key == "sk-x"
    assert mdl.model == "mistral-ocr-latest"


def test_mistral_ocr_model_reads_raw_secret_key():
    from rag.llm.ocr_model import MistralOcrModel

    mdl = MistralOcrModel(key="sk-raw", model_name="mistral-ocr-latest")
    assert mdl.api_key == "sk-raw"


def test_mistral_ocr_model_preserves_flat_string_api_key():
    # regression: a flat config whose "api_key" is a plain string (not a nested
    # config object) must keep the key rather than discard it.
    from rag.llm.ocr_model import MistralOcrModel
    import json

    key = json.dumps({"api_key": "sk-flat", "mistral_ocr_base_url": "https://api.mistral.ai/v1"})
    mdl = MistralOcrModel(key=key, model_name="mistral-ocr-latest")
    assert mdl.api_key == "sk-flat"
    assert mdl.base_url.rstrip("/").endswith("api.mistral.ai/v1")
