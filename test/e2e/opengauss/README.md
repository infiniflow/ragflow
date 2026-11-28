# openGauss document engine end-to-end tests

The suite creates and removes its fixtures through the RAGFlow Go HTTP API. It also queries openGauss to verify that chunks were persisted in the configured schema and removed after deletion.

## Running the suite

Configure the document engine as described in the [deployment guide](../../../docker/README.md#opengauss-datavec-document-engine). Start the Go server with a working ingestion worker, object store, message broker, cache and embedding model. Prepare two ordinary accounts in separate tenants. The owner account must be able to create datasets, skill spaces and memories and use the selected embedding model.

Supply the following variables to the test process through your normal credentials mechanism.

| Variable | Value |
| --- | --- |
| `RAGFLOW_E2E_URL` | Base URL of the Go service, excluding `/api/v1` |
| `RAGFLOW_E2E_OWNER_AUTH` | Authorization header value for the resource owner |
| `RAGFLOW_E2E_GUEST_AUTH` | Authorization header value for the second account |
| `RAGFLOW_E2E_EMBEDDING_MODEL` | Embedding model reference available to the owner |
| `RAGFLOW_E2E_OPENGAUSS_DSN` | Connection string for an account that can read the document engine tables, with the same database and schema search path as the server |

Run from the repository root. The suite skips when `RAGFLOW_E2E_URL` is unset.

```bash
bash build.sh --test-e2e -count=1 -v -run '^TestOpenGaussHTTP$' ./test/e2e/opengauss
```

## Coverage

| Scenario | Assertions |
| --- | --- |
| Documents and chunks | Cross-dataset keyword, vector and hybrid retrieval return the expected chunk and dataset IDs. Document and metadata filters narrow the results. Pagination, edits, rejected inputs and availability changes preserve the expected rows. Deleting a chunk or an entire dataset leaves its sibling dataset searchable. A second tenant cannot read private chunks or search the dataset. |
| Asynchronous ingestion | A text upload reaches the completed state. The parsed content is searchable, and deleting the document removes its chunks from openGauss. |
| Skills | Keyword, vector and hybrid queries report the requested search mode and return the indexed skill. Replacing a version returns only the new version. In the same space, deleting `guide/setup` preserves `guide_setup` and its version. Deleting both leaves the search empty. |
| Raw memory | Stored messages can be read and retrieved by session. The suite checks access across tenants, status changes and forgetting a message. |
| Concurrent chunk creation | Four requests add separate chunks to one document. The suite verifies the list total, each returned ID, its content and its database row. |

To check persistence across a service restart, create a separate skill space, index a versioned skill and record its ID, version and search mode. Restart the database with its data volume and restart the Go service. Repeat the same query against the same space, compare the three values, then delete the space.
