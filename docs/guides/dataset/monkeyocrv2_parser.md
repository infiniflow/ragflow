# MonkeyOCRv2-Parsing parser

RAGFlow can select `MonkeyOCRv2-Parsing` as a PDF layout parser. Configure the
service URL with `MONKEYOCRV2_SERVER_URL` (for example,
`http://monkeyocrv2:7861`). RAGFlow posts `multipart/form-data` to
`POST /parse` using a repeatable `files` field and `start_page_id`/`end_page_id`
form fields (zero-based, end exclusive).

The service response is the ZIP binary itself (`Content-Type: application/zip`).
The adapter extracts each document directory, reads MonkeyOCRv2 native
`jsons/*.json` layout records (`label`, `content`, `bbox`, `page_num`), and
converts them to RAGFlow section/table tuples. Markdown, images, and
`all_results.json` remain available in the extracted artifact directory.

Unlike the MinerU adapter, no MinerU-specific environment variables or model
provider are required; only the HTTP endpoint is configured.

## Verification

The integration was exercised against a live MonkeyOCRv2-Parsing service backed
by the local vLLM instance (`127.0.0.1:8888`). The request contained these two
files from `MonkeyOCRv2/images_test`:

| file | response entries | result |
| --- | ---: | --- |
| `table.png` | 4 | parsed Markdown, JSON layout and content-list |
| `exampaper.jpg` | 8 | parsed Markdown, JSON layout, content-list and 3 extracted images |

The batch request returned HTTP 200 in 5.90 s and a 50,366-byte ZIP. The raw
result metadata is saved as
`output/ragflow_monkeyocrv2_images_test.json` in the MonkeyOCRv2-Parsing
workspace. The parser unit test covers ZIP conversion and page-range handling:

```bash
python -m pytest -q test/unit_test/deepdoc/parser/test_monkeyocrv2_parser.py
```

For an isolated check, use a Python 3.11 virtual environment with `beartype`,
`pytest`, `requests`, `pillow`, `python-docx`, `markdown`, `numpy`, and
`pdfplumber` installed. The live test additionally requires the MonkeyOCRv2
service and vLLM endpoint to be running.

The adapter conversion test was also executed inside a Docker container
(`python:3.11-slim`) with the beartype-enabled test environment mounted in:

```text
docker parser integration: PASS
sections=[('hello', '@@2\\t1\\t30\\t2\\t40##')]
tables=[((None, 'a|b'), [(0, 0, 0, 10, 10)])]
```
