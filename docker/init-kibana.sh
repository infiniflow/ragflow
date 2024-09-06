#!/bin/bash

# unset http proxy which maybe set by docker daemon
export http_proxy=""; export https_proxy=""; export no_proxy=""; export HTTP_PROXY=""; export HTTPS_PROXY=""; export NO_PROXY=""

echo "Elasticsearch built-in user: elastic:${ELASTIC_PASSWORD}"

# Wait Elasticsearch be healthy
while true; do
    response=$(curl -s -v -w "\n%{http_code}" -u "elastic:${ELASTIC_PASSWORD}" "http://es01:9200")
    exit_code=$?
    status=$(echo "$response" | tail -n1)
    if [ $exit_code -eq 0 ] && [ "$status" = "200" ]; then
        echo "Elasticsearch is healthy"
        break
    else
        echo "Elasticsearch is unhealthy: $exit_code $status"
        echo "$response"
        sleep 5
    fi
done

# Create new role with all privileges to all indices
# https://www.elastic.co/guide/en/elasticsearch/reference/current/security-privileges.html#privileges-list-indices
echo "Going to create Elasticsearch role own_indices with all privileges to all indices"
while true; do
    response=$(curl -s -v -w "\n%{http_code}" -u "elastic:${ELASTIC_PASSWORD}" -X POST http://es01:9200/_security/role/own_indices -H 'Content-Type: application/json' -d '{"indices": [{"names": ["*"], "privileges": ["all"]}]}')
    exit_code=$?
    status=$(echo "$response" | tail -n1)
    if [ $exit_code -eq 0 ] && [ "$status" = "200" ]; then
        echo "Elasticsearch role own_indices created"
        break
    else
        echo "Elasticsearch role own_indices failure: $exit_code $status"
        echo "$response"
        sleep 5
    fi
done

echo "Elasticsearch role own_indices:"
curl -u "elastic:${ELASTIC_PASSWORD}" -X GET "http://es01:9200/_security/role/own_indices"
echo ""

PAYLOAD="{\"password\": \"${KIBANA_PASSWORD}\", \"roles\": [\"kibana_admin\", \"kibana_system\", \"own_indices\"], \"full_name\": \"${KIBANA_USER}\", \"email\": \"${KIBANA_USER}@example.com\"}"

echo "Going to create Elasticsearch user ${KIBANA_USER}: ${PAYLOAD}"

# Create new user
while true; do
    response=$(curl -s -v -w "\n%{http_code}" -u "elastic:${ELASTIC_PASSWORD}" -X POST http://es01:9200/_security/user/${KIBANA_USER} -H "Content-Type: application/json" -d "${PAYLOAD}")
    exit_code=$?
    status=$(echo "$response" | tail -n1)
    if [ $exit_code -eq 0 ] && [ "$status" = "200" ]; then
        echo "Elasticsearch user ${KIBANA_USER} created"
        break
    else
        echo "Elasticsearch user ${KIBANA_USER} failure: $exit_code $status"
        echo "$response"
        sleep 5
    fi
done

echo "Elasticsearch user ${KIBANA_USER}:"
curl -u "elastic:${ELASTIC_PASSWORD}" -X GET "http://es01:9200/_security/user/${KIBANA_USER}"
echo ""

exit 0
