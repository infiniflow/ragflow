from common.metadata_utils import ingestion_parser_config, legacy_ingestion_metadata_config


def test_modular_metadata_is_flattened_for_python_ingestion():
    user_fields = [{"key": "author", "type": "string"}]
    built_in = [{"key": "document_name", "type": "string"}]
    config = {"metadata": {"enabled": True, "metadata": user_fields, "built_in_metadata": built_in}}

    assert legacy_ingestion_metadata_config(config) == (True, user_fields, built_in)
    assert legacy_ingestion_metadata_config({"metadata": {**config["metadata"], "enabled": False}}) == (False, user_fields, built_in)


def test_flat_metadata_remains_compatible():
    config = {"enable_metadata": True, "metadata": [{"key": "author"}], "built_in_metadata": [{"key": "title"}]}
    assert legacy_ingestion_metadata_config(config) == (True, config["metadata"], config["built_in_metadata"])
    assert legacy_ingestion_metadata_config({}) == (False, [], [])


def test_task_normalization_does_not_change_stored_modular_config():
    fields = [{"key": "author", "type": "string"}]
    built_in = [{"key": "document_name", "type": "string"}]
    modular = {"enabled": True, "metadata": fields, "built_in_metadata": built_in}
    stored = {"metadata": modular, "Parser:General": {"chunk_token_num": 512}}
    normalized = ingestion_parser_config(stored)
    assert normalized == {
        "enable_metadata": True,
        "metadata": fields,
        "built_in_metadata": built_in,
        "Parser:General": {"chunk_token_num": 512},
    }
    assert stored["metadata"] == modular
    assert "enable_metadata" not in stored


def test_task_normalization_keeps_existing_flat_or_schema_metadata():
    schema = {"type": "object", "properties": {"author": {"type": "string"}}}
    flat = {"enable_metadata": True, "metadata": schema}
    assert ingestion_parser_config(flat)["metadata"] == schema
    assert ingestion_parser_config(flat)["enable_metadata"] is True


def test_modular_metadata_regression_on_unpatched_main():
    """A worker must not mistake a modular metadata object for a field list."""
    config = {"metadata": {"enabled": True, "metadata": [{"key": "author"}], "built_in_metadata": []}}
    normalized = ingestion_parser_config(config)
    assert normalized["enable_metadata"] is True
    assert normalized["metadata"] == [{"key": "author"}]
