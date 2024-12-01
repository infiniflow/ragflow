curl --location 'http://127.0.0.1/api/v1/chats/7896ac36aef011efa8e70242ac120003/completions' \
--header 'Content-Type: application/json' \
--header 'Authorization: Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm' \
--data '
     {
          "question": "你好，你是谁?",
          "stream": true
     }'