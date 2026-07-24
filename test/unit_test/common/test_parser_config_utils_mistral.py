from common.parser_config_utils import normalize_layout_recognizer


def test_mistral_ocr_suffix_normalized():
    raw = "mistral-ocr-latest@inst@Mistral OCR"
    layout, model_name = normalize_layout_recognizer(raw)
    assert layout == "Mistral OCR"
    assert model_name == raw


def test_mistral_ocr_suffix_case_insensitive():
    raw = "mistral-ocr-latest@inst@MISTRAL OCR"
    layout, model_name = normalize_layout_recognizer(raw)
    assert layout == "Mistral OCR"
    assert model_name == raw


def test_plain_mistral_not_captured():
    # a pixtral vision model on the multi-type Mistral factory must NOT be
    # dirverted to OCR
    layout, model_name = normalize_layout_recognizer("pixtral-large-latest@inst@Mistral")
    assert layout == "pixtral-large-latest@inst@Mistral"
    assert model_name is None
