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
