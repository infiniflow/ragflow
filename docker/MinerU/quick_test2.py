#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Quick test script for MinerU API using requests"""

import requests
import json
import os

pdf_path = '自然资源统一调查监测现状图建设的若干探索_韩爱惠.pdf'

print('文件大小:', os.path.getsize(pdf_path), 'bytes')

url = 'http://127.0.0.1:8000/file_parse'

with open(pdf_path, 'rb') as f:
    files = {'files': f}
    
    print('发送请求中...')
    response = requests.post(url, files=files, timeout=300)
    
    print('状态码:', response.status_code)
    print('响应头:', dict(response.headers))
    print('响应体长度:', len(response.text))
    
    if response.status_code == 200:
        data = response.json()
        print('成功!')
        print('数据类型:', type(data))
        if isinstance(data, dict):
            print('键:', list(data.keys())[:10])
    else:
        print('错误响应:', response.text[:1000])
