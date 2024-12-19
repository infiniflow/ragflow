# use http.client -----------------------------------
import http.client

conn = http.client.HTTPConnection("127.0.0.1")
payload = ''
headers = {
  'Authorization': 'Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm'
}
conn.request("GET", "/api/v1/datasets?name=api_create_dataset_python", payload, headers)
res = conn.getresponse()
data = res.read()
print(data.decode("utf-8"))

# use requests--------------------------------------
# import requests

# url = "http://127.0.0.1/api/v1/datasets?name=api_create_dataset_python"

# payload = {}
# headers = {
#   'Authorization': 'Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm'
# }

# response = requests.request("GET", url, headers=headers, data=payload)

# print(response.text)