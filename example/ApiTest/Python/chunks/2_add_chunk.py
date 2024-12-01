import requests
import json

url = "http://127.0.0.1/api/v1/datasets/8a85ab34ad5311ef98b00242ac120003/documents/501e387aadf411ef922e0242ac120003/chunks"

payload = json.dumps({
  "content": "add some content"
})
headers = {
  'Content-Type': 'application/json',
  'Authorization': 'Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm'
}

response = requests.request("POST", url, headers=headers, data=payload)

print(response.text)
