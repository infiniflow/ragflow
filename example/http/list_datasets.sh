#!/bin/bash
#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

# Check for jq
if ! command -v jq &> /dev/null; then
    echo "jq could not be found, please install it to run this example."
    exit 1
fi

# Usage
usage() {
    echo "Usage: $0 <host> <api_key> [--page PAGE] [--page-size PAGE_SIZE] [--orderby FIELD] [--desc true|false] [--name NAME] [--id ID]"
    echo ""
    echo "Required arguments:"
    echo "  host        RAGFlow server address (e.g. http://localhost:9380)"
    echo "  api_key     Your API key"
    echo ""
    echo "Optional arguments:"
    echo "  --page        Page number"
    echo "  --page-size   Number of items per page"
    echo "  --orderby     Field to order by"
    echo "  --desc        Sort in descending order (true/false)"
    echo "  --name        Filter by dataset name"
    echo "  --id          Filter by dataset ID"
    exit 1
}

# Parse required arguments
if [ $# -lt 2 ]; then
    usage
fi

HOST="$1"
API_KEY="$2"
shift 2

# Parse optional arguments
PAGE=""
PAGE_SIZE=""
ORDERBY=""
DESC=""
NAME=""
ID=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --page)
            PAGE="$2"; shift 2 ;;
        --page-size)
            PAGE_SIZE="$2"; shift 2 ;;
        --orderby)
            ORDERBY="$2"; shift 2 ;;
        --desc)
            DESC="$2"; shift 2 ;;
        --name)
            NAME="$2"; shift 2 ;;
        --id)
            ID="$2"; shift 2 ;;
        -h|--help)
            usage ;;
        *)
            echo "Unknown option: $1"
            usage ;;
    esac
done

# Build query string
QUERY=""
[ -n "$PAGE" ] && QUERY="${QUERY}&page=${PAGE}"
[ -n "$PAGE_SIZE" ] && QUERY="${QUERY}&page_size=${PAGE_SIZE}"
[ -n "$ORDERBY" ] && QUERY="${QUERY}&orderby=${ORDERBY}"
[ -n "$DESC" ] && QUERY="${QUERY}&desc=${DESC}"
[ -n "$NAME" ] && QUERY="${QUERY}&name=${NAME}"
[ -n "$ID" ] && QUERY="${QUERY}&id=${ID}"

# Remove leading '&'
QUERY=$(echo "$QUERY" | sed 's/^&//')
[ -n "$QUERY" ] && QUERY="?${QUERY}"

# Store, print and make the request
CURL_CMD=(curl -s --request GET \
    --url "${HOST}/api/v1/datasets${QUERY}" \
    --header "Authorization: Bearer ${API_KEY}")
echo "${CURL_CMD[@]}"
RESPONSE=$("${CURL_CMD[@]}")

# Check for errors
if echo "$RESPONSE" | jq -e '.code != 0' > /dev/null 2>&1; then
    echo "Error: $(echo "$RESPONSE" | jq -r '.message // .')"
    exit 1
fi

# Output result
echo "$RESPONSE" | jq .
