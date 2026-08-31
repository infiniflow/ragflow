from common.parser_config_utils import normalize_layout_recognizer


def test_normalize_layout_recognizer_monkeyocr_suffix():
    assert normalize_layout_recognizer("my-llm@my-instance@my-provider@monkeyocr") == (
        "MonkeyOCR",
        "my-llm@my-instance@my-provider@monkeyocr",
    )
