curl --location --request PUT 'http://127.0.0.1/api/v1/datasets/919f28e0ac9b11efabe50242ac120003' \
--header 'Content-Type: application/json' \
--header 'Authorization: Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm' \
--data '
     {
          "name": "api_updated_dataset",
          "chunk_method":"manual",
          "embedding_model":"embedding-3"
     }'