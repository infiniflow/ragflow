from common.metadata_utils import convert_conditions


def test_convert_conditions_supports_new_schema():
    metadata_condition = {
        "logic": "and",
        "conditions": [
            {"key": "author", "op": "=", "value": "alice"},
            {"key": "score", "op": ">=", "value": "10"},
        ],
    }

    assert convert_conditions(metadata_condition) == [
        {"key": "author", "op": "=", "value": "alice"},
        {"key": "score", "op": "≥", "value": "10"},
    ]


def test_convert_conditions_supports_legacy_schema():
    metadata_condition = {
        "logic": "or",
        "conditions": [
            {"name": "author", "comparison_operator": "is", "value": "alice"},
            {"name": "score", "comparison_operator": "!=", "value": "10"},
        ],
    }

    assert convert_conditions(metadata_condition) == [
        {"key": "author", "op": "=", "value": "alice"},
        {"key": "score", "op": "≠", "value": "10"},
    ]
