# Qdrant Backend Notes

This document describes the Qdrant document-store backend for RAGFlow: what it supports, how it is wired, how to run it, and what differences remain versus Elasticsearch.

## Overview

The Qdrant backend adds:

- a Qdrant storage adapter for the existing synchronous document-store interface
- `DOC_ENGINE=qdrant` routing and settings support
- upstream Docker wiring in `docker/.env`, `docker/service_conf.yaml.template`, `docker/docker-compose-base.yml`, and `docker/docker-compose.yml`
- dense retrieval
- optional sparse hybrid retrieval
- admin config and health awareness

The adapter is intended to replace Elasticsearch as the document/vector backend for normal RAGFlow usage. It does not try to reproduce Elasticsearch analyzer or BM25 behavior exactly.

## Design

### Storage model

- one Qdrant collection per tenant for chunk data
- one Qdrant collection per tenant for document metadata
- original RAGFlow document and chunk IDs remain in payload
- internal Qdrant point IDs are stored as deterministic UUIDs because Qdrant only accepts unsigned integers or UUIDs

### Vector layout

- named dense vector: `dense`
- named sparse vector: `sparse`

Collection schema shape:

```python
vectors_config = {
    "dense": models.VectorParams(
        size=dimension,
        distance=models.Distance.COSINE,
    ),
    # Future ColPali / ColQwen extension point:
    # "page_dense": models.VectorParams(
    #     size=page_dimension,
    #     distance=models.Distance.COSINE,
    #     multivector_config=models.MultiVectorConfig(
    #         comparator=models.MultiVectorComparator.MAX_SIM,
    #     ),
    # ),
}

sparse_vectors_config = {
    "sparse": models.SparseVectorParams(
        index=models.SparseIndexParams(
            on_disk=False,
        ),
    ),
}
```

### Retrieval

- dense-only retrieval works with the `dense` named vector
- hybrid retrieval uses dense prefetch + sparse prefetch + reciprocal-rank fusion
- when no sparse model is configured, the backend silently falls back to dense-only retrieval
- a text payload index is created on chunk text for simple keyword-oriented matching

Hybrid query shape:

```python
result = client.query_points(
    collection_name=collection,
    prefetch=[
        models.Prefetch(
            query=models.NamedVector(
                name="dense",
                vector=dense_query,
            ),
            using="dense",
            limit=similarity_top_k,
            filter=query_filter,
        ),
        models.Prefetch(
            query=models.NamedSparseVector(
                name="sparse",
                vector=models.SparseVector(
                    indices=sparse_query.indices,
                    values=sparse_query.values,
                ),
            ),
            using="sparse",
            limit=similarity_top_k,
            filter=query_filter,
        ),
    ],
    query=models.FusionQuery(
        fusion=models.Fusion.RRF,
    ),
    limit=top_k,
    with_payload=True,
    with_vectors=False,
)
```

### Compatibility notes

- missing `available_int` is treated as available to match existing backend behavior
- future inserts default `available_int=1`
- admin-side config and health checks understand `qdrant`
- backend error text should refer to the active backend instead of hardcoded `Elasticsearch/Infinity`

## Setup

There are two supported setup modes:

1. standard upstream Docker setup from `docker/`
2. custom deployment using your own compose/config stack

### Standard upstream Docker setup

Relevant files:

- `docker/.env`
- `docker/service_conf.yaml.template`
- `docker/docker-compose-base.yml`
- `docker/docker-compose.yml`

Minimal dense-only startup:

```bash
cd docker
DOC_ENGINE=qdrant docker compose -f docker-compose.yml up -d
```

Dense + sparse startup:

```bash
cd docker
DOC_ENGINE=qdrant QDRANT_SPARSE_MODEL=token docker compose -f docker-compose.yml up -d
```

Important environment variables:

```env
DOC_ENGINE=qdrant
QDRANT_HOST=qdrant
QDRANT_HTTP_PORT=6333
QDRANT_GRPC_PORT=6334
QDRANT_SPARSE_MODEL=
QDRANT_SPARSE_VOCAB_SIZE=131072
QDRANT_TEXT_INDEX_TOKENIZER=multilingual
```

Set:

- `QDRANT_SPARSE_MODEL=` for dense-only
- `QDRANT_SPARSE_MODEL=token` for the built-in sparse path used here

### Custom deployment setup

If you do not use the repo Docker entrypoint directly, you still need:

1. `DOC_ENGINE=qdrant`
2. a `qdrant:` block in your runtime `service_conf.yaml`

Example:

```yaml
qdrant:
  host: qdrant
  http_port: 6333
  grpc_port: 6334
  https: false
  prefer_grpc: false
  timeout: 10
  sparse_model: ''
  sparse_vocab_size: 131072
  text_index_tokenizer: multilingual
```

For dense-only:

```yaml
sparse_model: ''
```

For hybrid:

```yaml
sparse_model: 'token'
```

## Validation

Recommended smoke-test order:

1. Start RAGFlow with `DOC_ENGINE=qdrant`
2. Create a fresh test dataset
3. Upload a small document
4. Wait for indexing to finish
5. Open the chunk list and confirm raw chunk content is shown
6. Create a chat attached to that dataset
7. Ask a question copied from a real chunk
8. Confirm the answer references the ingested content

Optional hybrid validation:

1. enable `sparse_model: 'token'`
2. restart the service
3. retry retrieval with keyword-heavy queries

## Troubleshooting

If ingestion fails with an error that a point ID is invalid:

- the backend is sending a non-UUID external RAGFlow ID directly to Qdrant
- Qdrant requires an unsigned integer or UUID point ID
- the adapter must keep the original RAGFlow ID in payload and use a deterministic UUID as the internal point ID

If chat returns no knowledge despite successful indexing:

- verify the conversation path is not applying an empty `doc_id` filter
- no selected documents should behave like `doc_ids=None`, not `doc_ids=[]`

If chunk text looks stemmed or truncated in the UI:

- verify the API is returning raw `content_with_weight`
- highlight/snippet text should be returned separately and must not replace the stored chunk text

If Qdrant starts correctly but admin logs report `Unknown configuration key: qdrant`:

- the admin config/status parser is missing Qdrant awareness
- add `qdrant` to the supported config keys in the admin config path

## Current limitations

These are intentional or currently accepted limitations:

- Qdrant is not a 1:1 Elasticsearch BM25/analyzer replacement
- SQL retrieval is unsupported for `DOC_ENGINE=qdrant`
- message store is unsupported for `DOC_ENGINE=qdrant`
- the adapter is synchronous because the current storage contract is synchronous
- ColPali / ColQwen visual RAG is not implemented here

## Future extension point

The collection schema and query path are prepared for a later multi-vector extension, but this work does not implement it. A future ColPali / ColQwen PR should:

- add a separate page- or image-level named vector
- enable multivector comparison for that field
- extend ingestion to attach page/image vectors
- extend retrieval and reranking so page-level visual hits can be returned cleanly
