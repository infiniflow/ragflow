# use http.client -----------------------------------
import http.client
import json

conn = http.client.HTTPConnection("127.0.0.1")
payload = json.dumps({
  "name": "api_updated_dataset_python",
  "chunk_method": "manual",
  "embedding_model": "embedding-3"
})
headers = {
  'Content-Type': 'application/json',
  'Authorization': 'Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm'
}
conn.request("PUT", "/api/v1/datasets/98ced1c0ad5811efadff0242ac120003", payload, headers)
res = conn.getresponse()
data = res.read()
print(data.decode("utf-8"))

# use requests--------------------------------------
# import requests
# import json

# url = "http://127.0.0.1/api/v1/datasets/919f28e0ac9b11efabe50242ac120003"

# payload = json.dumps({
#   "name": "api_updated_dataset",
#   "chunk_method": "manual",
#   "embedding_model": "embedding-3"
# })
# headers = {
#   'Content-Type': 'application/json',
#   'Authorization': 'Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm'
# }

# response = requests.request("PUT", url, headers=headers, data=payload)

# print(response.text)
