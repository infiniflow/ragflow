import requests
import json

url = "http://127.0.0.1/api/v1/chats/7896ac36aef011efa8e70242ac120003/completions"

payload = json.dumps({
  "question": "你好，你是谁?",
  "stream": True
})
headers = {
  'Content-Type': 'application/json',
  'Authorization': 'Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm'
}

response = requests.request("POST", url, headers=headers, data=payload)

print(response.text)
