from api.utils.model_utils import calculate_model_type, normalize_model_types
from common.constants import ModelTypeBinary


def test_normalize_model_types_collapses_legacy_capability_aliases():
    assert normalize_model_types(["speech2text", "asr"]) == ["asr"]
    assert normalize_model_types(["chat", "speech2text", "image2text"]) == [
        "chat",
        "asr",
        "vision",
    ]


def test_normalize_model_types_ignores_invalid_metadata():
    assert normalize_model_types(1) == []  # type: ignore[arg-type]


def test_calculate_model_type_accepts_legacy_aliases():
    assert calculate_model_type(["speech2text", "image2text"]) == (ModelTypeBinary.ASR.value | ModelTypeBinary.VISION.value)
