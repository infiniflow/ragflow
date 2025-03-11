
# read file content from local、remote or dataset
from abc import ABC
import ast
import logging
import re

import requests

from agent.component.base import ComponentBase, ComponentParamBase
from RestrictedPython import safe_builtins, compile_restricted_exec


class FileReaderParam(ComponentParamBase):
    """
    Define the Coder component parameters.
    """
    def __init__(self):
        super().__init__()
        self.type = 'remote'  # [ local, remote, dataset]

    def check(self):
        self.check_valid_value(self.type, "File type", ['local', 'remote', 'dataset'])


class FileReader(ComponentBase, ABC):
    component_name = "FileReader"

    # def get_dependent_components(self):
    #     inputs = self.get_input_elements()
    #     cpnts = set([i["key"] for i in inputs if i["key"].lower().find("answer") < 0 and i["key"].lower().find("begin") < 0])
    #     return list(cpnts).append(super().get_dependent_components())        

    def _run(self, history, **kwargs):
        path = ''
        header = {}
        for query in self._param.query:
            qName = query['name']
            if qName == 'path':
                path = str(query['value'])
            elif qName.startswith('header'):
                _, hKey = header.split('.', 1)
                setattr(header, hKey, query['value'])

        if not path:
            raise Exception("path is empty")
        
        # 如果是远程文件，header中加上常用的header
        if self._param.type == 'remote':
            header['User-Agent'] = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36'
            header['Accept'] = 'text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9'
            header['Accept-Language'] = 'zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6'
            header['Accept-Encoding'] = 'gzip, deflate, br'
            header['Connection'] = 'keep-alive'
            header['Upgrade-Insecure-Requests'] = '1'
            header['Cache-Control'] = 'max-age=0'

        fileBytes = None
        if self._param.type == 'local':
            fileBytes = self.readerLocalFile(path)
        elif self._param.type == 'remote':
            fileBytes = self.readerRemoteFile(path, header)
        else:
            raise Exception('Unknown file type')

        return FileReader.be_output(fileBytes)
    
    def readerRemoteFile(self, url, header):
        try:
            response = requests.get(url, headers=header)
            if response.status_code == 200:
                return response.content
            else:
                return None
        except Exception as e:
            raise e

    def readerLocalFile(self, path):
        try:
            with open(path, 'r') as f:
                return f.read()
        except Exception as e:
            raise e
    