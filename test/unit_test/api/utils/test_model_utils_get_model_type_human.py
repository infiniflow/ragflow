from api.utils.model_utils import calculate_model_type, get_model_type_human, normalize_model_types
from common.constants import ModelTypeBinary


def test_get_model_type_human_single_type():
    """Single model type returns its name."""
    assert get_model_type_human(ModelTypeBinary.CHAT.value) == ["chat"]
    assert get_model_type_human(ModelTypeBinary.EMBEDDING.value) == ["embedding"]
    assert get_model_type_human(ModelTypeBinary.ASR.value) == ["asr"]


def test_get_model_type_human_combined_types():
    """Combined model type returns all matching names."""
    combined = ModelTypeBinary.CHAT.value | ModelTypeBinary.EMBEDDING.value
    result = get_model_type_human(combined)
    assert "chat" in result
    assert "embedding" in result


def test_get_model_type_human_zero():
    """Zero model type returns empty list."""
    assert get_model_type_human(0) == []


def test_get_model_type_human_all_types():
    """All model types combined returns all names."""
    all_types = sum(mt.value for mt in ModelTypeBinary)
    result = get_model_type_human(all_types)
    assert len(result) == len(ModelTypeBinary)
    for mt in ModelTypeBinary:
        assert mt.name.lower() in result


def test_get_model_type_human_does_not_mutate_binary_constants():
    """Calling get_model_type_human should not mutate ModelTypeBinary values."""
    original_values = {mt.name: mt.value for mt in ModelTypeBinary}
    get_model_type_human(ModelTypeBinary.CHAT.value)
    get_model_type_human(0)
    for mt in ModelTypeBinary:
        assert mt.value == original_values[mt.name]


def test_normalize_model_types_handles_string_input():
    """Single string input is normalized to a list."""
    assert normalize_model_types("chat") == ["chat"]


def test_normalize_model_types_handles_empty_list():
    """Empty list returns empty list."""
    assert normalize_model_types([]) == []


def test_normalize_model_types_handles_none_input():
    """None input returns empty list."""
    assert normalize_model_types(None) == []


def test_normalize_model_types_deduplicates():
    """Duplicate entries are deduplicated."""
    assert normalize_model_types(["chat", "chat"]) == ["chat"]


def test_normalize_model_types_preserves_order():
    """Order of first occurrence is preserved."""
    assert normalize_model_types(["embedding", "chat", "asr"]) == ["embedding", "chat", "asr"]


def test_calculate_model_type_empty_list():
    """Empty list returns 0."""
    assert calculate_model_type([]) == 0


def test_calculate_model_type_single_string():
    """Single string input returns its value."""
    assert calculate_model_type("chat") == ModelTypeBinary.CHAT.value


def test_calculate_model_type_uses_lowercase_lookup():
    """Model type names are matched via lowercase lookup keys."""
    assert calculate_model_type(["chat"]) == ModelTypeBinary.CHAT.value


def test_calculate_model_type_ignores_unknown():
    """Unknown model type names are ignored."""
    assert calculate_model_type(["nonexistent"]) == 0
    assert calculate_model_type(["chat", "nonexistent"]) == ModelTypeBinary.CHAT.value