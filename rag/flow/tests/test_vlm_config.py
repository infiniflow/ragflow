from rag.flow.parser.vlm_config import get_vlm_config


def test_nested_vlm_config_is_preferred():
    assert get_vlm_config({"vlm": {"llm_id": "new-model"}, "llm_id": "legacy-model"}) == {
        "llm_id": "new-model"
    }


def test_legacy_llm_id_fills_missing_nested_config():
    assert get_vlm_config({"vlm": {"temperature": 0.2}, "llm_id": "legacy-model"}) == {
        "temperature": 0.2,
        "llm_id": "legacy-model",
    }


def test_legacy_llm_id_is_supported_without_nested_config():
    assert get_vlm_config({"llm_id": "legacy-model"}) == {"llm_id": "legacy-model"}


def test_missing_vlm_config_stays_empty():
    assert get_vlm_config({}) == {}
