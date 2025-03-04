curl --location --request DELETE 'http://127.0.0.1/api/v1/datasets/8a85ab34ad5311ef98b00242ac120003/documents/501e387aadf411ef922e0242ac120003/chunks' \
--header 'Content-Type: application/json' \
--header 'Authorization: Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm' \
--data '
     {
          "chunk_ids": ["a74cc41dd2abc8d32a24bcc370e73412"]
     }'