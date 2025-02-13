import requests
import json

url = "http://127.0.0.1/api/v1/chats"

payload = json.dumps({
  "ids": [
    "b8f7957aabd411efafbd0242ac120006",
    "b056fb6eabd811efb6000242ac120006"
  ]
})
headers = {
  'Content-Type': 'application/json',
  'Authorization': 'Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm'
}

response = requests.request("DELETE", url, headers=headers, data=payload)

print(response.text)
