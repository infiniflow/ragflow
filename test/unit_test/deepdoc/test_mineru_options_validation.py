import pytest
from deepdoc.parser.mineru_parser import MinerUParser, MinerUParseOptions
from deepdoc.parser.mineru_parser import MinerUBackend, MinerULanguage, MinerUParseMethod
from pathlib import Path


def test_validate_start_end_pages_invalid():
    parser = MinerUParser(mineru_api="http://test-api")
    opts = MinerUParseOptions(start_page=10, end_page=5)
    with pytest.raises(ValueError):
        parser._validate_parse_options(opts)


def test_validate_batch_size_clamping_and_defaults():
    parser = MinerUParser(mineru_api="http://test-api")

    opts = MinerUParseOptions(batch_size=0)
    parser._validate_parse_options(opts)
    assert opts.batch_size == 30

    opts = MinerUParseOptions(batch_size=10000)
    parser._validate_parse_options(opts)
    assert opts.batch_size == 500

    opts = MinerUParseOptions(batch_size=50)
    parser._validate_parse_options(opts)
    assert opts.batch_size == 50


def test_strict_mode_coercion_and_type():
    parser = MinerUParser(mineru_api="http://test-api")
    opts = MinerUParseOptions(strict_mode="yes")
    parser._validate_parse_options(opts)
    assert isinstance(opts.strict_mode, bool)
    assert opts.strict_mode is True


def test_run_mineru_api_raises_on_invalid_options():
    parser = MinerUParser(mineru_api="http://test-api")
    opts = MinerUParseOptions(start_page=10, end_page=5)
    with pytest.raises(RuntimeError):
        parser._run_mineru_api(Path("/tmp/fake.pdf"), Path("/tmp/out"), opts)
