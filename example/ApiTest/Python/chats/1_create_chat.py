import requests
import json

url = "http://127.0.0.1/api/v1/chats"

payload = json.dumps({
  "dataset_ids": [
    "8a85ab34ad5311ef98b00242ac120003"
  ],
  "name": "my test chat"
})
headers = {
  'Content-Type': 'application/json',
  'Authorization': 'Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm'
}

response = requests.request("POST", url, headers=headers, data=payload)

print(response.text)
