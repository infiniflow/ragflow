#!/bin/bash
set -euo pipefail

HOST_ADDRESS="${RAGFLOW_HOST_ADDRESS:-http://localhost:9380}"
API_KEY="${RAGFLOW_API_KEY:-ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm}"

command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }
TEMP_FILE=$(mktemp)
DATASET_ID=""
DOC_ID=""
cleanup() {
    rm -f "$TEMP_FILE"
    [ -n "$DOC_ID" ] && curl -fsS -X DELETE "$HOST_ADDRESS/api/v1/datasets/$DATASET_ID/documents" \
        -H 'Content-Type: application/json' -H "Authorization: Bearer $API_KEY" \
        -d "{\"ids\":[\"$DOC_ID\"]}" >/dev/null || true
    [ -n "$DATASET_ID" ] && curl -fsS -X DELETE "$HOST_ADDRESS/api/v1/datasets" \
        -H 'Content-Type: application/json' -H "Authorization: Bearer $API_KEY" \
        -d "{\"ids\":[\"$DATASET_ID\"]}" >/dev/null || true
}
trap cleanup EXIT
printf '%s\n' 'RAGFlow is an open-source RAG engine.' >"$TEMP_FILE"

DATASET_ID=$(curl -fsS -X POST "$HOST_ADDRESS/api/v1/datasets" \
    -H 'Content-Type: application/json' -H "Authorization: Bearer $API_KEY" \
    -d '{"name":"document_http_example"}' | jq -er '.data.id')
DOC_ID=$(curl -fsS -X POST "$HOST_ADDRESS/api/v1/datasets/$DATASET_ID/documents" \
    -H "Authorization: Bearer $API_KEY" -F "file=@$TEMP_FILE;filename=sample.txt" | jq -er '.data[0].id')

curl -fsS "$HOST_ADDRESS/api/v1/datasets/$DATASET_ID/documents?id=$DOC_ID" \
    -H "Authorization: Bearer $API_KEY" | jq .
curl -fsS -X PATCH "$HOST_ADDRESS/api/v1/datasets/$DATASET_ID/documents/$DOC_ID" \
    -H 'Content-Type: application/json' -H "Authorization: Bearer $API_KEY" \
    -d '{"name":"renamed_sample.txt"}' | jq .
curl -fsS -X POST "$HOST_ADDRESS/api/v1/datasets/$DATASET_ID/documents/parse" \
    -H 'Content-Type: application/json' -H "Authorization: Bearer $API_KEY" \
    -d "{\"document_ids\":[\"$DOC_ID\"]}" | jq .
for _ in $(seq 1 60); do
    STATUS=$(curl -fsS "$HOST_ADDRESS/api/v1/datasets/$DATASET_ID/documents?id=$DOC_ID" \
        -H "Authorization: Bearer $API_KEY" | jq -er '.data.docs[0].run')
    echo "Parsing status: $STATUS"
    case "$STATUS" in
        DONE|3) break ;;
        FAIL|4|CANCEL|2) echo "Parsing ended with status $STATUS" >&2; exit 1 ;;
    esac
    sleep 2
done
case "$STATUS" in DONE|3) ;; *) echo "Parsing timed out" >&2; exit 1 ;; esac
