# use http.client -----------------------------------
import http.client
import json

conn = http.client.HTTPConnection("127.0.0.1")
payload = json.dumps({
  "name": "api_create_dataset_python",
  "language": "Chinese",
  "embedding_model": "text-embedding-v3"
})
headers = {
  'Content-Type': 'application/json',
  'Authorization': 'Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm'
}
conn.request("POST", "/api/v1/datasets", payload, headers)
res = conn.getresponse()
data = res.read()
print(data.decode("utf-8"))

# use requests--------------------------------------

# import requests
# import json

# url = "http://127.0.0.1/api/v1/datasets"

# payload = json.dumps({
#   "name": "api_create_dataset_python",
#   "language": "Chinese",
#   "embedding_model": "text-embedding-v3"
# })
# headers = {
#   'Content-Type': 'application/json',
#   'Authorization': 'Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm'
# }

# response = requests.request("POST", url, headers=headers, data=payload)

# print(response.text)
