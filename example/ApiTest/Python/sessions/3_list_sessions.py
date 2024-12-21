import requests

url = "http://127.0.01/api/v1/chats/36734bf8aee011ef9eb50242ac120003/sessions?name=change session name&id=b745827eaee411efa65f0242ac120003"

payload = {}
headers = {
  'Authorization': 'Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm'
}

response = requests.request("GET", url, headers=headers, data=payload)

print(response.text)
