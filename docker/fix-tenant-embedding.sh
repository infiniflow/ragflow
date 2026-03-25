#!/bin/bash
# Fix: ensure TEI embedding model has @Builtin suffix in tenant table
# Required for RagFlow v0.24.0 with local TEI embedding (Builtin provider)
# Run this once after fresh deployment if datasets fail with @None error
echo "Applying tenant embedding model fix..."
docker exec docker-mysql-1 mysql -u root -pinfini_rag_flow rag_flow \
  -e "UPDATE tenant SET embd_id='BAAI/bge-small-en-v1.5@Builtin' \
      WHERE embd_id='BAAI/bge-small-en-v1.5';" 2>/dev/null
echo "Done. Current tenant embedding models:"
docker exec docker-mysql-1 mysql -u root -pinfini_rag_flow rag_flow \
  -e "SELECT name, embd_id FROM tenant;" 2>/dev/null
