import requests


class BaseApi:
    def __init__(self, user_key, base_url, authorization_header):
        pass

    def post(self, path, param, stream=False):
        res = requests.post(url=self.api_url + path, json=param, headers=self.authorization_header, stream=stream)
        return res

    def put(self, path, param, stream=False):
        res = requests.put(url=self.api_url + path, json=param, headers=self.authorization_header, stream=stream)
        return res

    def get(self, path, params=None):
        res = requests.get(url=self.api_url + path, params=params, headers=self.authorization_header)
        return res

    def delete(self, path, params):
        res = requests.delete(url=self.api_url + path, params=params, headers=self.authorization_header)
        return res




