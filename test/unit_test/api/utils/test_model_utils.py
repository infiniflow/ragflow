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


def test_get_model_type_human_returns_empty_for_string_model_type():
    # Defensive guard for the partial-migration case in #19569: a varchar
    # model_type that survived the in-place upgrade must not turn the whole
    # /api/v1/models response into a 500.
    assert get_model_type_human("ocr") == []


def test_get_model_type_human_returns_empty_for_none():
    assert get_model_type_human(None) == []  # type: ignore[arg-type]


def test_get_model_type_human_returns_empty_for_bool():
    # bool is a subclass of int in Python; the '&' would silently evaluate to
    # truthy values without an explicit guard.
    assert get_model_type_human(True) == []  # type: ignore[arg-type]


def test_get_model_type_human_still_returns_names_for_int():
    # Sanity check that the defensive guard does not change the int happy path.
    chat_only = ModelTypeBinary.CHAT.value
    assert get_model_type_human(chat_only) == ["chat"]
    chat_plus_vision = ModelTypeBinary.CHAT.value | ModelTypeBinary.VISION.value
    assert get_model_type_human(chat_plus_vision) == ["chat", "vision"]
