import os
from pathlib import Path

import pytest

from deepdoc.parser.mineru_parser import MinerUParseOptions, MinerUParser


def test_validate_start_end_enforces_strict_less_than():
    opts = MinerUParseOptions(start_page=4, end_page=4)

    p = MinerUParser()
    with pytest.raises(ValueError):
        p._validate_parse_options(opts)


def test_parse_pdf_pages_parameter_applies_to_options(monkeypatch, tmp_path):
    parser = MinerUParser()

    captured = {}

    def fake_run_mineru(self, input_path: Path, output_dir: Path, options, callback=None):
        captured['options'] = options
        # return a dummy output dir (Path)
        return Path(tmp_path / "out")

    monkeypatch.setattr(MinerUParser, '_run_mineru', fake_run_mineru)
    # Avoid reading output files for this unit test; we only care about options mapping
    monkeypatch.setattr(MinerUParser, '_read_output', lambda *args, **kwargs: [])

    # Provide a binary and parser_config with pages [[5,15]] (1-based)
    binary_pdf = b"%PDF-1.4"
    parser_cfg = {"pages": [[5, 15]], "mineru_batch_size": 25}

    # Create a temporary file path; parse_pdf will write the binary to a temp file
    out_sections, out_tables = parser.parse_pdf("/tmp/fake.pdf", binary_pdf, parser_config=parser_cfg, output_dir=str(tmp_path))

    opts = captured.get('options')
    assert opts is not None, "_run_mineru was not called or options not captured"
    assert opts.start_page == 4
    assert opts.end_page == 15
    assert opts.batch_size == 25
