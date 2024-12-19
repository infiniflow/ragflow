# request-----------------------------------------------
import requests

url = "http://127.0.0.1/api/v1/datasets/842986a8ad5711ef8b530242ac120003/documents"

payload = {}
files=[
  ('file',('hd.txt',open('D:/资料/ragflow/hd.txt','rb'),'text/plain')),
  ('file',('数字中国建设整体布局规划.txt',open('D:/资料/ragflow/数字中国建设整体布局规划.txt','rb'),'text/plain'))
]
headers = {
  'Authorization': 'Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm'
}

response = requests.request("POST", url, headers=headers, data=payload, files=files)

print(response.text)
