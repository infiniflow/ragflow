from common.metadata_utils import metadata_schema


def test_metadata_schema_returns_empty_when_input_is_none():
    # returns empty schema when metadata definition is missing
    assert metadata_schema(None) == {}


def test_metadata_schema_builds_string_field():
    # builds a default string field schema with description
    schema = metadata_schema([{"key": "author", "description": "Author name"}])
    assert schema == {
        "type": "object",
        "properties": {
            "author": {
                "type": "string",
                "description": "Author name",
            }
        },
        "additionalProperties": False,
    }


def test_metadata_schema_builds_list_and_enum_on_items():
    # builds array schema and applies enum constraints on array items
    schema = metadata_schema(
        [
            {
                "key": "tags",
                "type": "list",
                "description": "Document tags",
                "enum": ["a", "b"],
            }
        ]
    )
    assert schema["properties"]["tags"] == {
        "type": "array",
        "description": "Document tags",
        "items": {"type": "string", "enum": ["a", "b"]},
    }


def test_metadata_schema_builds_number_and_time_fields():
    # builds typed schemas for number and date-time metadata fields
    schema = metadata_schema(
        [
            {"key": "score", "type": "number", "description": "Ranking score", "enum": [1, 2]},
            {"key": "publish_at", "type": "time", "description": "Publish time"},
        ]
    )
    assert schema["properties"]["score"] == {
        "type": "number",
        "description": "Ranking score",
        "enum": [1, 2],
    }
    assert schema["properties"]["publish_at"] == {
        "type": "string",
        "format": "date-time",
        "description": "Publish time",
    }


def test_metadata_schema_skips_items_without_key():
    # ignores invalid metadata items that do not provide a key
    schema = metadata_schema([{"type": "string", "description": "no-key"}])
    assert schema == {
        "type": "object",
        "properties": {},
        "additionalProperties": False,
    }
