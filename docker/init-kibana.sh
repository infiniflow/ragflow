#!/bin/bash

# 等待 Elasticsearch 啟動
until curl -u "elastic:${ELASTIC_PASSWORD}" -s http://es01:9200 >/dev/null; do
  echo "等待 Elasticsearch 啟動..."
  sleep 5
done


echo "使用者: elastic:${ELASTIC_PASSWORD}"



PAYLOAD="{
  \"password\" : \"${KIBANA_PASSWORD}\",
  \"roles\" : [ \"kibana_admin\",\"kibana_system\" ],
  \"full_name\" : \"${KIBANA_USER}\",
  \"email\" : \"${KIBANA_USER}@example.com\"
}"
echo "新用戶帳戶: $PAYLOAD"

# 創建新用戶帳戶
curl -X POST "http://es01:9200/_security/user/${KIBANA_USER}" \
-u "elastic:${ELASTIC_PASSWORD}" \
-H "Content-Type: application/json" \
-d "$PAYLOAD"s

echo "新用戶帳戶已創建"

exit 0
