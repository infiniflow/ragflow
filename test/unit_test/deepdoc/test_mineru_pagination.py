import pytest
from unittest.mock import patch, Mock, mock_open
from pathlib import Path
from deepdoc.parser.mineru_parser import MinerUParser, MinerUParseOptions


def test_run_mineru_api_includes_pagination_params():
    parser = MinerUParser(mineru_api="http://test-api")
    options = MinerUParseOptions(batch_size=25, start_page=10, end_page=50, exif_correction=True, strict_mode=False)

    mock_response = Mock()
    mock_response.status_code = 200
    mock_response.headers = {"Content-Type": "application/zip"}
    mock_response.content = b"fake zip content"

    with patch("requests.post", return_value=mock_response) as mock_post, \
         patch("tempfile.mkdtemp", return_value="/tmp/test_output"), \
         patch("tempfile.mkstemp", return_value=(None, "/tmp/test_file.zip")), \
         patch("os.path.exists", return_value=True), \
         patch("builtins.open", mock_open()), \
         patch.object(MinerUParser, "_extract_zip_no_root") as mock_extract:
        # Call the API runner; should not raise
        parser._run_mineru_api(Path("/tmp/fake.pdf"), Path("/tmp/out"), options)

        assert mock_post.called
        _, kwargs = mock_post.call_args
        data = kwargs.get("data")
        assert data is not None
        assert data.get("start_page_id") == 10
        assert data.get("end_page_id") == 50
        assert data.get("batch_size") == 25
        assert data.get("exif_correction") is True
        assert data.get("strict_mode") is False


def test_parse_pdf_pages_parameter_applies_to_options():
    parser = MinerUParser(mineru_api="http://test-api")

    # Capture the options passed into _run_mineru
    captured = {}

    def fake_run(api_self, input_path, output_dir, options, callback=None):
        captured["options"] = options
        return Path("/tmp/out_dir")

    parser_cfg = {
        "layout_recognize": "MinerU",
        "pages": [[5, 15]],  # user sets pages (1-based): should translate to start_page=4, end_page=15
    }

    with patch.object(MinerUParser, "_run_mineru", fake_run), \
         patch.object(MinerUParser, "__images__", lambda *a, **k: None), \
         patch.object(MinerUParser, "_read_output", return_value=[]):
        # Note: parse_pdf signature expects (filepath, binary, callback=None, *, output_dir=...)
        # Pass non-empty bytes so parser treats it as binary input and writes a temp PDF
        parser.parse_pdf("/tmp/fake.pdf", b"%PDF-1.4", parser_config=parser_cfg, output_dir="/tmp/out")

    assert "options" in captured
    opts = captured["options"]
    assert opts.start_page == 4
    assert opts.end_page == 15
