curl --location 'http://127.0.0.1/api/v1/retrieval' \
--header 'Content-Type: application/json' \
--header 'Authorization: Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm' \
--data '{
        "question": "some questions?",
        "dataset_ids": ["8a85ab34ad5311ef98b00242ac120003"],
        "document_ids": ["501e387aadf411ef922e0242ac120003"]
}'