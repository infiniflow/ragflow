#!/usr/bin/env bash
set -eu

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dockerfile="${repo_root}/Dockerfile"
production_stage="$(awk '
    /^FROM base AS production$/ { in_production = 1; next }
    in_production && /^FROM[[:space:]]/ { exit }
    in_production
' "${dockerfile}")"
expected_copy='COPY internal/rag/res/ner.json internal/rag/res/synonym.json ./internal/rag/res/'

if ! grep -Fqx "${expected_copy}" <<< "${production_stage}"; then
    printf 'FAIL: production image must copy ner.json and synonym.json into ./internal/rag/res/\n' >&2
    exit 1
fi

printf 'PASS: production image includes the Go NLP dictionaries\n'
