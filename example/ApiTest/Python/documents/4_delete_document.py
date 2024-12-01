# http.client
import http.client
import json

conn = http.client.HTTPConnection("127.0.0.1")
payload = json.dumps({
  "ids": [
    "51fb16eead6911ef957d0242ac120003"
  ]
})
headers = {
  'Content-Type': 'application/json',
  'Authorization': 'Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm'
}
conn.request("DELETE", "/api/v1/datasets/8a85ab34ad5311ef98b00242ac120003/documents", payload, headers)
res = conn.getresponse()
data = res.read()
print(data.decode("utf-8"))