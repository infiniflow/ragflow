def test_mistral_cv_registers_in_registry():
    from rag.llm import CvModel

    assert "Mistral" in CvModel, sorted(CvModel.keys())


def test_mistral_cv_defaults_to_pixtral_and_mistral_base_url():
    from rag.llm.cv_model import MistralCV

    cv = MistralCV(key="sk-test")
    assert cv.model_name == "pixtral-12b-2409"
    assert cv.base_url.rstrip("/").endswith("api.mistral.ai/v1")
