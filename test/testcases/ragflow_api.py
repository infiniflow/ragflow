#!/usr/bin/env python
# -*- coding: utf-8 -*-
# Copyright (c) GDS, Inc. All rights reserved.
# @Time    : 2025/7/29 0:01
# @Author  : tonymhl
# @email   : hongli.ma@gds-services.com
# @Version : Python3
# @Project : ragflow
# @File    : ragflow_api.py
# @Software: PyCharm


import json
import requests

BASE_URL ='http://172.25.9.200:80/v1/'  # http://localhost,/v1/api/
API_KEY='ragflow-k4ZGJjYjg0NmI2NTExZjA4NDRmYTYwNj'

def start_conversation():
    url = BASE_URL + 'api/new_conversation'
    headers = {"'Content-Type": "application/json",
               'Authorization': 'Bearer ' + API_KEY}
    response = requests.get(url, headers=headers)
    conversation_id = None
    msg = None
    if response.status_code == 200:
        content = response.json()
        conversation_id = content['data']['id']
        msg = content['data']['message']  # content role
        print("start_conversation:", conversation_id, msg)
    else:
        print(f"Request failed with status code: {response.status_code}")
    return response.status_code, conversation_id, msg

def get_answer(conversation_id, msg, quote=False, stream=True):
    url = BASE_URL + 'api/completion'
    params = {
        "conversation_id": conversation_id,  # chat ID
        "messages": [{"role": "user", "content": msg}],  # message content
        "quote": False,
        "stream": False,
    }
    print(params)
    headers_json = {"Content-Type": "application/json",
                    'Authorization': 'Bearer ' + API_KEY}
    try:
        response = requests.post(url=url, headers=headers_json, data=json.dumps(params))
        print(response.json())
        response.raise_for_status()  # Raises an HTTPError for bad responses (4xx and 5xx)
    except requests.exceptions.HTTPError as http_err:
        print(f"HTTP error occurred: {http_err}")
        return response.status_code, None, None
    except Exception as err:
        print(f"An error occurred: {err}")
        return None, None, None
    try:
        content = response.json()
        data = content.get('data', None)
        retcode = content.get('retcode', None)
        retmsg = content.get('retmsg', None)
        if data:
            answer = data. get('answer', None)
        else:
            answer = None
    except ValueError:
        print("Failed to parse JSON response")
        return response.status_code, None, None

    return response.status_code, answer, retmsg

def chat():
    status_code, conversation_id, msg = start_conversation()
    # conversation id = input("Enter conversation ID: ")
    print("Chatbot initialized. Type 'exit' to end the conversation.")

    while True:
        user_message = input("You: ")
        if user_message.lower() == 'exit':
            print("Ending the conversation.")
            break
        status_code, answer, retmsg = get_answer(conversation_id, user_message)
        if status_code is not None and status_code == 200 and answer:
            print(f"Chatbot: {answer}")
        elif retmsg:
            print(f"Failed to get a response from the chatbot. Error message: {retmsg}")
        else:
            print("Failed to get a response from the chatbot.")

if __name__ == '__main__':
    chat()