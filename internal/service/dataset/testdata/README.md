# Dataset Tag API Contract Checks

These checks cover the existing Go routes listed in #15240:

- `GET /api/v1/datasets/{dataset_id}/tags`: JSON tag/count pairs, including an empty array when no tags exist.
- `GET /api/v1/datasets/tags/aggregation`: query actual stores even when `doc_num` is zero; preserve tenant grouping and access checks; reject more than 100 nonempty IDs.
- `PUT /api/v1/datasets/{dataset_id}/tags`: preserve the original match, replacement, and response strings. Python strips only the removal operand.

All three use the Python success envelope (`code`, `data`) and error envelope (`code`, `message`). Internal failures return code 102 and `Internal server error`; the Go server retains details in its log.

## Local Regression Tests

With the native dependencies described in `internal/development.md` installed:

```bash
bash build.sh --test -run 'TestDatasetService(ListTags|AggregateTags|RenameTag)|TestDatasetsHandler.*Tags|TestDatasetsHandlerRenameTag' ./internal/service/dataset ./internal/handler
```

The service tests use SQLite and fake document engines. The handler tests send HTTP requests through Gin with fake service boundaries. These unit tests do not require a running backend service.

The reference check executes the original Python functions extracted with `ast`, replacing database and network I/O with fakes. It does not import or start the full Python server:

```bash
python internal/service/dataset/testdata/python_tags_contract.py
```

References: `api/apps/restful_apis/dataset_api.py`, `api/apps/services/dataset_api_service.py`, `rag/nlp/search.py:all_tags`, and `api/utils/pagination_utils.py:validate_rest_api_ids`.

The existing Go dataset-ID normalization remains in place. Result ordering across tenant groups is not guaranteed. The native document-engine queries still need real-service integration testing.

## API Smoke Checks

Use a running Go server, an authorized test user, and a disposable dataset with a tagged chunk. Set `RAGFLOW_URL`, `RAGFLOW_TOKEN`, and `DATASET_ID` before running these commands:

```bash
curl --fail-with-body -sS -H "Authorization: Bearer $RAGFLOW_TOKEN" \
  "$RAGFLOW_URL/api/v1/datasets/$DATASET_ID/tags"
# Expected data shape: [["finance",2],["urgent",1]]. Counts depend on the fixture.

curl --fail-with-body -sS -H "Authorization: Bearer $RAGFLOW_TOKEN" \
  "$RAGFLOW_URL/api/v1/datasets/tags/aggregation?dataset_ids=$DATASET_ID"
# Expected data shape: [{"value":"finance","count":2}, ...].

curl --fail-with-body -sS -X PUT -H "Authorization: Bearer $RAGFLOW_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"from_tag":"old-tag ","to_tag":" new-tag "}' \
  "$RAGFLOW_URL/api/v1/datasets/$DATASET_ID/tags"
# Expected: {"code":0,"data":{"from":"old-tag ","to":" new-tag "}}.

curl --fail-with-body -sS -H "Authorization: Bearer $RAGFLOW_TOKEN" \
  "$RAGFLOW_URL/api/v1/datasets/tags/aggregation"
# Expected: {"code":102,"message":"Lack of dataset_ids in query parameters"}.
```

Repeat listing with a user who cannot access the disposable dataset: the response must contain `no authorization`, with no tags. Verify list and aggregate responses for a dataset with an absent chunk store, and verify the exact renamed values in the document engine. Application errors use HTTP 200, so inspect the JSON `code` as well as curl's exit status.
