from api.db.joint_services.model_type_resolution import metadata_supports_model_type


def test_multimodal_metadata_supports_chat_when_model_type_is_legacy_image2text():
    metadata = {"model_type": "image2text", "tags": "LLM,CHAT,IMAGE2TEXT"}

    assert metadata_supports_model_type(metadata, "chat")


def test_model_type_list_supports_chat_directly():
    metadata = {"model_type": ["image2text", "chat"], "tags": "LLM,IMAGE2TEXT"}

    assert metadata_supports_model_type(metadata, "chat")


def test_unrelated_metadata_does_not_support_chat():
    metadata = {"model_type": "embedding", "tags": "TEXT EMBEDDING"}

    assert not metadata_supports_model_type(metadata, "chat")
