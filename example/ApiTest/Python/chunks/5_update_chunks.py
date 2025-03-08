import requests
import json

url = "http://127.0.0.1/api/v1/datasets/8a85ab34ad5311ef98b00242ac120003/documents/501e387aadf411ef922e0242ac120003/chunks/f4444516b61f95adfdd293173177be4a"

payload = json.dumps({
  "content": "some update content",
  "important_keywords": [
    "名字由来"
  ]
})
headers = {
  'Content-Type': 'application/json',
  'Authorization': 'Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm'
}

response = requests.request("PUT", url, headers=headers, data=payload)

print(response.text)
