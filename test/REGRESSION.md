# Regression lanes

Run each lane separately: the live API conftest selects priority markers and
must not change the selection of unit tests or P0 document requirements.

| Lane | Command from repository root | Boundary |
| --- | --- | --- |
| Python unit | `python run_tests.py -i` | In-process dependencies and fixtures |
| Browser result assertions | `python -m pytest test/unit_test/playwright test/unit_test/api/apps/services/test_search_consistency_assertions.py` | Reject missing requests, empty/error/truncated answers, and invalid search results |
| Document coverage | `python -m pytest test/unit_test/api/apps/business_documents test/unit_test/api/apps/restful_apis/test_business_document_api_contract.py --cov=api/apps/business_documents --cov-branch --cov-fail-under=79` | Domain, worker, evidence, export, authorization and API contracts; combined statement/branch coverage |
| Document requirements | `python -m pytest test/evals/business_documents/test_golden_dialogue_harness.py test/evals/business_documents/test_live_quality_scorer.py` | Scripted golden cases and scorer; optional live-model smoke can skip |
| PostgreSQL races | `python -m pytest test/integration/test_business_document_postgres.py` | Requires `BUSINESS_DOCUMENT_TEST_POSTGRES_DSN`; separate connections and temporary schema with verified cleanup |
| Frontend | `cd web` then `pnpm test --runInBand` | Jest; measured global floors for statements, lines, functions and branches |
| Isolated browser | `python test/run_browser_regression.py --browser chromium` | Built SPA, intercepted API, no live backend; also accepts `firefox` and `webkit` |
| Live browser | `python -m pytest test/playwright/e2e/test_dataset_upload_parse.py test/playwright/e2e/test_next_apps_chat.py test/playwright/e2e/test_next_apps_search.py` | Disposable complete stack, configured model provider and `RAGFLOW_BASE_URL` |
| Web/admin API | `python -m pytest test/testcases/test_web_api` and separately `python -m pytest test/testcases/test_admin_api` | Disposable stack; SDK installed, provider configuration and mapped admin endpoint |
| Go | `bash test/run_go_regression.sh` | Linux CI after native build; all packages, disposable MinIO and tokenizer dictionaries |

On Windows, use `.venv/Scripts/python.exe` for `python`. Build the SPA with
`pnpm build` in `web` before the isolated browser lane, and install the selected
browser with `python -m playwright install chromium` (or Firefox/WebKit).

Browser journeys execute all their steps in one test and use independent page
and state fixtures. Missing prerequisites fail rather than count as skipped
steps. Unhandled page exceptions fail the test. Failure evidence is written
under `test/playwright/artifacts/<browser>`.

The isolated browser lane covers document role controls and conflict handling,
navigation permissions, wrong password, HTTP/envelope session expiry, logout,
empty upload selection, removal/cancel, rejection and retry. Its upload 400/413
responses are mocked: these prove frontend behavior, not backend file parsing
or size enforcement. Cancel is dismissal before submission, not an in-flight
abort. Mocked roles do not replace server authorization tests.

The live search journey inserts known content and requires its document and
snippet in both the response and UI, then removes its document. The chat
journey requires a correlated completed request, successful stream events,
nonempty answer and a newly rendered assistant message.

Never run live suites against shared customer data. Existing live API fixtures
can delete tenant datasets/chats. CI uses disposable Compose projects; the
PostgreSQL and MinIO race/storage lanes isolate and clean their own data.

`browser-regression.yml` runs the isolated browser matrix on pull requests.
`tests.yml` and `sep-tests.yml` run requirements, PostgreSQL races, the document
coverage gate, Web/admin APIs and live browser journeys alongside their existing
unit/SDK/REST lanes. Go coverage is no longer reduced by package exclusions.

The coverage floors prevent regression from the measured baseline; they are
not a claim that every application route or every negative case is covered.
