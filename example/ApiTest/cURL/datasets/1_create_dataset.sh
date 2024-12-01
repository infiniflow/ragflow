curl --location 'http://127.0.0.1/api/v1/datasets' \
--header 'Content-Type: application/json' \
--header 'Authorization: Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm' \
--data '{
      "name": "api_create_dataset" ,
      "language": "Chinese",
      "embedding_model":"text-embedding-v3"
      }'