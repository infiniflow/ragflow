from pathlib import Path

from deepdoc.parser.mineru_parser import MinerUParser, MinerUParseOptions


def test_run_mineru_api_batches_calls_single_batch(monkeypatch, tmp_path):
    parser = MinerUParser()

    # Create a dummy pdf file to satisfy existence check
    pdf_file = tmp_path / "fake.pdf"
    pdf_file.write_bytes(b"%PDF-1.4")

    out_dir = tmp_path / "out"
    out_dir.mkdir()

    calls = []

    def fake_get_total_pages(self, path):
        assert Path(path) == pdf_file
        return 10

    def fake_single_batch(self, input_path, output_dir, options, start_page, end_page, callback=None):
        calls.append((start_page, end_page))
        # create a fake content list file returned by the batch
        tmp = output_dir / f"batch_{start_page}_{end_page}"
        tmp.mkdir(exist_ok=True)
        (tmp / f"{Path(input_path).stem}_content_list.json").write_text("[]")
        return tmp

    # monkeypatch methods
    monkeypatch.setattr(MinerUParser, "_get_total_pages", fake_get_total_pages)
    monkeypatch.setattr(MinerUParser, "_run_mineru_api_single_batch", fake_single_batch)
    monkeypatch.setattr(MinerUParser, "_read_output", lambda *args, **kwargs: [])

    options = MinerUParseOptions(batch_size=4, start_page=None, end_page=None)

    merged_path = parser._run_mineru_api(pdf_file, out_dir, options)

    # Expect 3 batches: 0-3,4-7,8-9
    assert calls == [(0, 3), (4, 7), (8, 9)]
    assert merged_path.exists()
