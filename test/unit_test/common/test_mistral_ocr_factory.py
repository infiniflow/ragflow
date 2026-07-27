import json
from pathlib import Path


def test_mistral_ocr_factory_present_and_ocr_tagged():
    repo_root = Path(__file__).resolve().parents[3]
    data = json.loads((repo_root / "conf" / "llm_factories.json").read_text())
    factories = {f["name"]: f for f in data["factory_llm_infos"]}
    assert "Mistral OCR" in factories, "Mistral OCR factory missing"
    fac = factories["Mistral OCR"]
    assert "OCR" in fac["tags"]
    # ships a default OCR model (like the other OCR factories) so it is usable
    # without manually adding one through the model provider page; the llm_name
    # is the real Mistral API id because the parser POSTs it verbatim to /v1/ocr.
    models = {m["llm_name"]: m for m in fac["llm"]}
    assert "mistral-ocr-latest" in models
    assert models["mistral-ocr-latest"]["model_type"] == "ocr"
    assert "OCR" in models["mistral-ocr-latest"]["tags"]


def test_mistral_ocr_factory_distinct_from_mistral():
    repo_root = Path(__file__).resolve().parents[3]
    data = json.loads((repo_root / "conf" / "llm_factories.json").read_text())
    names = [f["name"] for f in data["factory_llm_infos"]]
    assert "Mistral" in names and "Mistral OCR" in names
