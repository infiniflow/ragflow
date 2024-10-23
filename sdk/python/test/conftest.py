import pytest
import requests
import string
import random



HOST_ADDRESS = 'http://127.0.0.1:9380'

def generate_random_email():
    return 'user_' + ''.join(random.choices(string.ascii_lowercase + string.digits, k=8))+'@1.com'

EMAIL = generate_random_email()
# password is "123"
PASSWORD='''ctAseGvejiaSWWZ88T/m4FQVOpQyUvP+x7sXtdv3feqZACiQleuewkUi35E16wSd5C5QcnkkcV9cYc8TKPTRZlxappDuirxghxoOvFcJxFU4ixLsD
fN33jCHRoDUW81IH9zjij/vaw8IbVyb6vuwg6MX6inOEBRRzVbRYxXOu1wkWY6SsI8X70oF9aeLFp/PzQpjoe/YbSqpTq8qqrmHzn9vO+yvyYyvmDsphXe
X8f7fp9c7vUsfOCkM+gHY3PadG+QHa7KI7mzTKgUTZImK6BZtfRBATDTthEUbbaTewY4H0MnWiCeeDhcbeQao6cFy1To8pE3RpmxnGnS8BsBn8w=='''

def get_email():
    return EMAIL

def register():
    url = HOST_ADDRESS + "/v1/user/register"
    name = "user"
    register_data = {"email":EMAIL,"nickname":name,"password":PASSWORD}
    res = requests.post(url=url,json=register_data)
    res = res.json()
    if res.get("retcode") != 0:
        raise Exception(res.get("retmsg"))

def login():
    url = HOST_ADDRESS + "/v1/user/login"
    login_data = {"email":EMAIL,"password":PASSWORD}
    response=requests.post(url=url,json=login_data)
    res = response.json()
    if res.get("retcode")!=0:
        raise Exception(res.get("retmsg"))
    auth = response.headers["Authorization"]
    return auth

@pytest.fixture(scope="session")
def get_api_key_fixture():
    register()
    auth = login()
    url = HOST_ADDRESS + "/v1/system/new_token"
    auth = {"Authorization": auth}
    response = requests.post(url=url,headers=auth)
    res = response.json()
    if res.get("retcode") != 0:
        raise Exception(res.get("retmsg"))
    return res["data"].get("token")

