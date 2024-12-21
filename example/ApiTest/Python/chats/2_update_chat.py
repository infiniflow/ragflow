import requests

url = "http://127.0.0.1/api/v1/chats?name=api_create_dataset&id=8a85ab34ad5311ef98b00242ac120003"

payload = {}
headers = {
  'Authorization': 'Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm'
}

response = requests.request("GET", url, headers=headers, data=payload)

print(response.text)
