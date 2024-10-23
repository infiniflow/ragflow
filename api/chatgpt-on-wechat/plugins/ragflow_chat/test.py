import requests
import json

# 配置参数
api_key = "ragflow-***"
host_address = "127.0.0.1:80"
user_id = "test_user"
user_input = "你好"

# 公共请求头
headers = {
    "Authorization": f"Bearer {api_key}",
    "Content-Type": "application/json"
}

# Step 1: 创建新的会话（注意 URL 中的 /v1）
url_new_conversation = f"http://{host_address}/v1/api/new_conversation"
params_new_conversation = {
    "user_id": user_id
}

print("Creating new conversation...")
response = requests.get(url_new_conversation, headers=headers, params=params_new_conversation)
print("Status Code:", response.status_code)
print("Response Content:", response.text)

if response.status_code == 200:
    try:
        data = response.json()
        if data.get("retcode") == 0:
            conversation_id = data["data"]["id"]
            print("Conversation ID:", conversation_id)
        else:
            print("Failed to create conversation:", data.get("retmsg"))
            exit(1)
    except Exception as e:
        print("Failed to parse response as JSON:", e)
        exit(1)
else:
    print("Failed to create conversation")
    exit(1)

# Step 2: 获取回复
url_completion = f"http://{host_address}/v1/api/completion"
payload_completion = {
    "conversation_id": conversation_id,
    "messages": [
        {
            "role": "user",
            "content": user_input
        }
    ],
    "quote": False,
    "stream": False
}

print("\nSending message to conversation...")
response = requests.post(url_completion, headers=headers, json=payload_completion)
print("Status Code:", response.status_code)
print("Response Content:", response.text)

if response.status_code == 200:
    try:
        data = response.json()
        if data.get("retcode") == 0:
            answer = data["data"]["answer"]
            print("Assistant's reply:", answer)
        else:
            print("Failed to get answer:", data.get("retmsg"))
    except Exception as e:
        print("Failed to parse response as JSON:", e)
else:
    print("Failed to get answer")
