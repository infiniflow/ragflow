from api.utils.model_utils import calculate_model_type, get_model_type_human, normalize_model_types
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


def test_get_model_type_human_with_integers():
    assert get_model_type_human(ModelTypeBinary.CHAT.value) == ["chat"]
    assert get_model_type_human(ModelTypeBinary.CHAT.value | ModelTypeBinary.VISION.value) == ["chat", "vision"]
    assert get_model_type_human(ModelTypeBinary.OCR.value) == ["ocr"]


def test_get_model_type_human_with_string_and_aliases():
    # Numeric strings (e.g. from MySQL driver/cursor without explicit casting)
    assert get_model_type_human("64") == ["ocr"]
    assert get_model_type_human("1") == ["chat"]
    # Legacy alias strings
    assert get_model_type_human("ocr") == ["ocr"]
    assert get_model_type_human("speech2text") == ["asr"]
    assert get_model_type_human("image2text") == ["vision"]


def test_get_model_type_human_with_empty_or_invalid():
    assert get_model_type_human(None) == []
    assert get_model_type_human("unknown_type") == []
    assert get_model_type_human(0) == []
    assert get_model_type_human(123.45) == []  # type: ignore[arg-type]
