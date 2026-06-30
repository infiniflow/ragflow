import pytest

from common.tag_feature_utils import parse_tag_features, validate_tag_features


def test_validate_tag_features_accepts_numeric_dict():
    assert validate_tag_features({"apple": 1, "banana": 2.5}) == {
        "apple": 1.0,
        "banana": 2.5,
    }


def test_validate_tag_features_rejects_string_payload():
    with pytest.raises(ValueError, match="object mapping string tags"):
        validate_tag_features('{"apple": 1.0}')


def test_validate_tag_features_rejects_non_finite_or_non_numeric_values():
    with pytest.raises(ValueError, match="finite numbers"):
        validate_tag_features({"apple": float("inf")})

    with pytest.raises(ValueError, match="finite numbers"):
        validate_tag_features({"apple": "1.0"})


def test_parse_tag_features_supports_legacy_python_literal_strings():
    assert parse_tag_features("{'apple': 2.0}", allow_python_literal=True) == {"apple": 2.0}


def test_parse_tag_features_ignores_executable_strings():
    payload = '{"apple": (__import__("time").sleep(1) or 1.0)}'
    assert parse_tag_features(payload, allow_python_literal=True) == {}
