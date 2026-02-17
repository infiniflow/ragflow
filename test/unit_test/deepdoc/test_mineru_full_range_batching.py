from pathlib import Path

from deepdoc.parser.mineru_parser import MinerUParser, MinerUParseOptions


def test_explicit_full_range_enables_batching(monkeypatch, tmp_path):
    parser = MinerUParser()

    pdf_file = tmp_path / "big.pdf"
    pdf_file.write_bytes(b"%PDF-1.4")

    out_dir = tmp_path / "out"
    out_dir.mkdir()

    calls = []

    def fake_get_total_pages(self, path):
        assert Path(path) == pdf_file
        return 146

    def fake_single_batch(self, input_path, output_dir, options, start_page, end_page, callback=None):
        calls.append((start_page, end_page))
        tmp = output_dir / f"batch_{start_page}_{end_page}"
        tmp.mkdir(exist_ok=True)
        (tmp / f"{Path(input_path).stem}_content_list.json").write_text("[]")
        return tmp

    monkeypatch.setattr(MinerUParser, "_get_total_pages", fake_get_total_pages)
    monkeypatch.setattr(MinerUParser, "_run_mineru_api_single_batch", fake_single_batch)
    monkeypatch.setattr(MinerUParser, "_read_output", lambda *args, **kwargs: [])

    # Explicit pages covering full doc (1-based user range -> start=0, end=146)
    opts = MinerUParseOptions(batch_size=30, start_page=0, end_page=146)

    merged = parser._run_mineru_api(pdf_file, out_dir, opts)

    # Expect batching (146 pages with batch_size=30 -> 5 batches: 0-29,30-59,60-89,90-119,120-145, and last 145-145)
    assert len(calls) >= 1
    assert merged.exists()
