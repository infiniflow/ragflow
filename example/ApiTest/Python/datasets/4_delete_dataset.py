# use http.client -----------------------------------
import http.client
import json

conn = http.client.HTTPConnection("127.0.0.1")
payload = json.dumps({
  "ids": [
    "009055e6ad5811ef82fa0242ac120003"
  ]
})
headers = {
  'Content-Type': 'application/json',
  'Authorization': 'Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm'
}
conn.request("DELETE", "/api/v1/datasets", payload, headers)
res = conn.getresponse()
data = res.read()
print(data.decode("utf-8"))


# use requests -----------------------------------

# import requests
# import json

# url = "http://127.0.0.1/api/v1/datasets"

# payload = json.dumps({
#   "ids": [
#     "919f28e0ac9b11efabe50242ac120003"
#   ]
# })
# headers = {
#   'Content-Type': 'application/json',
#   'Authorization': 'Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm'
# }

# response = requests.request("DELETE", url, headers=headers, data=payload)

# print(response.text)
