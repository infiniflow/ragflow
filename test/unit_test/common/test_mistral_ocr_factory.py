import json
from pathlib import Path


def test_mistral_ocr_factory_present_and_ocr_tagged():
    repo_root = Path(__file__).resolve().parents[3]
    data = json.loads((repo_root / "conf" / "llm_factories.json").read_text())
    factories = {f["name"]: f for f in data["factory_llm_infos"]}
    assert "Mistral OCR" in factories, "Mistral OCR factory missing"
    fac = factories["Mistral OCR"]
    assert "OCR" in fac["tags"]
    assert fac["llm"] == []


def test_mistral_ocr_factory_distinct_from_mistral():
    repo_root = Path(__file__).resolve().parents[3]
    data = json.loads((repo_root / "conf" / "llm_factories.json").read_text())
    names = [f["name"] for f in data["factory_llm_infos"]]
    assert "Mistral" in names and "Mistral OCR" in names
