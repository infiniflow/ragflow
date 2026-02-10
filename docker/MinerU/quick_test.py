#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Quick test script for MinerU API"""

import urllib.request
import json

pdf_path = '自然资源统一调查监测现状图建设的若干探索_韩爱惠.pdf'

with open(pdf_path, 'rb') as f:
    file_content = f.read()

print('文件大小:', len(file_content), 'bytes')

boundary = '----WebKitFormBoundary' + 'A'*16

body = b''
body += b'------' + boundary.encode() + b'\r\n'
body += b'Content-Disposition: form-data; name="files"; filename="' + pdf_path.encode('utf-8') + b'"\r\n'
body += b'Content-Type: application/pdf\r\n\r\n'
body += file_content + b'\r\n'
body += b'------' + boundary.encode() + b'--\r\n'

url = 'http://127.0.0.1:8000/file_parse'
req = urllib.request.Request(url, data=body, method='POST')
req.add_header('Content-Type', 'multipart/form-data; boundary=----WebKitFormBoundaryAAAAAAAAAAAAAA')

try:
    with urllib.request.urlopen(req, timeout=300) as resp:
        body_response = resp.read().decode('utf-8')
        print('成功!')
        print('响应长度:', len(body_response))
        data = json.loads(body_response)
        print('类型:', type(data))
        if isinstance(data, dict):
            print('键:', list(data.keys())[:10])
except urllib.error.HTTPError as e:
    print('HTTP错误:', e.code)
    print('响应体:', e.read().decode('utf-8')[:1000])
except Exception as e:
    print('错误:', type(e).__name__, e)
