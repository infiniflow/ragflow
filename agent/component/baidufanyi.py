#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
import random
from abc import ABC
import requests
from agent.component.base import ComponentBase, ComponentParamBase
from hashlib import md5


class BaiduFanyiParam(ComponentParamBase):
    """
    Define the BaiduFanyi component parameters.
    """

    def __init__(self):
        super().__init__()
        self.appid = "xxx"
        self.secret_key = "xxx"
        self.trans_type = 'translate'
        self.parameters = []
        self.source_lang = 'auto'
        self.target_lang = 'auto'
        self.domain = 'finance'

    def check(self):
        self.check_empty(self.appid, "BaiduFanyi APPID")
        self.check_empty(self.secret_key, "BaiduFanyi Secret Key")
        self.check_valid_value(self.trans_type, "Translate type", ['translate', 'fieldtranslate'])
        self.check_valid_value(self.source_lang, "Source language",
                               ['auto', 'zh', 'en', 'yue', 'wyw', 'jp', 'kor', 'fra', 'spa', 'th', 'ara', 'ru', 'pt',
                                'de', 'it', 'el', 'nl', 'pl', 'bul', 'est', 'dan', 'fin', 'cs', 'rom', 'slo', 'swe',
                                'hu', 'cht', 'vie'])
        self.check_valid_value(self.target_lang, "Target language",
                               ['auto', 'zh', 'en', 'yue', 'wyw', 'jp', 'kor', 'fra', 'spa', 'th', 'ara', 'ru', 'pt',
                                'de', 'it', 'el', 'nl', 'pl', 'bul', 'est', 'dan', 'fin', 'cs', 'rom', 'slo', 'swe',
                                'hu', 'cht', 'vie'])
        self.check_valid_value(self.domain, "Translate field",
                               ['it', 'finance', 'machinery', 'senimed', 'novel', 'academic', 'aerospace', 'wiki',
                                'news', 'law', 'contract'])


class BaiduFanyi(ComponentBase, ABC):
    component_name = "BaiduFanyi"

    def _run(self, history, **kwargs):

        ans = self.get_input()
        ans = " - ".join(ans["content"]) if "content" in ans else ""
        if not ans:
            return BaiduFanyi.be_output("")

        try:
            source_lang = self._param.source_lang
            target_lang = self._param.target_lang
            appid = self._param.appid
            salt = random.randint(32768, 65536)
            secret_key = self._param.secret_key

            if self._param.trans_type == 'translate':
                sign = md5((appid + ans + salt + secret_key).encode('utf-8')).hexdigest()
                url = 'http://api.fanyi.baidu.com/api/trans/vip/translate?' + 'q=' + ans + '&from=' + source_lang + '&to=' + target_lang + '&appid=' + appid + '&salt=' + salt + '&sign=' + sign
                headers = {"Content-Type": "application/x-www-form-urlencoded"}
                response = requests.post(url=url, headers=headers).json()

                if response.get('error_code'):
                    BaiduFanyi.be_output("**Error**:" + response['error_msg'])

                return BaiduFanyi.be_output(response['trans_result'][0]['dst'])
            elif self._param.trans_type == 'fieldtranslate':
                domain = self._param.domain
                sign = md5((appid + ans + salt + domain + secret_key).encode('utf-8')).hexdigest()
                url = 'http://api.fanyi.baidu.com/api/trans/vip/fieldtranslate?' + 'q=' + ans + '&from=' + source_lang + '&to=' + target_lang + '&appid=' + appid + '&salt=' + salt + '&domain=' + domain + '&sign=' + sign
                headers = {"Content-Type": "application/x-www-form-urlencoded"}
                response = requests.post(url=url, headers=headers).json()

                if response.get('error_code'):
                    BaiduFanyi.be_output("**Error**:" + response['error_msg'])

                return BaiduFanyi.be_output(response['trans_result'][0]['dst'])

        except Exception as e:
            BaiduFanyi.be_output("**Error**:" + str(e))
    
