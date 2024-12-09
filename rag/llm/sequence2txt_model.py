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
import requests
from openai.lib.azure import AzureOpenAI
import io
from abc import ABC
from openai import OpenAI
import json
from rag.utils import num_tokens_from_string
import base64
import re


class Base(ABC):
    def __init__(self, key, model_name):
        pass

    def transcription(self, audio, **kwargs):
        transcription = self.client.audio.transcriptions.create(
            model=self.model_name,
            file=audio,
            response_format="text"
        )
        return transcription.text.strip(), num_tokens_from_string(transcription.text.strip())

    def audio2base64(self, audio):
        if isinstance(audio, bytes):
            return base64.b64encode(audio).decode("utf-8")
        if isinstance(audio, io.BytesIO):
            return base64.b64encode(audio.getvalue()).decode("utf-8")
        raise TypeError("The input audio file should be in binary format.")


class GPTSeq2txt(Base):
    def __init__(self, key, model_name="whisper-1", base_url="https://api.openai.com/v1"):
        if not base_url:
            base_url = "https://api.openai.com/v1"
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name


class QWenSeq2txt(Base):
    def __init__(self, key, model_name="paraformer-realtime-8k-v1", **kwargs):
        import dashscope
        dashscope.api_key = key
        self.model_name = model_name

    def transcription(self, audio, format):
        from http import HTTPStatus
        from dashscope.audio.asr import Recognition

        recognition = Recognition(model=self.model_name,
                                  format=format,
                                  sample_rate=16000,
                                  callback=None)
        result = recognition.call(audio)

        ans = ""
        if result.status_code == HTTPStatus.OK:
            for sentence in result.get_sentence():
                ans += sentence.text.decode('utf-8') + '\n'
            return ans, num_tokens_from_string(ans)

        return "**ERROR**: " + result.message, 0


class AzureSeq2txt(Base):
    def __init__(self, key, model_name, lang="Chinese", **kwargs):
        self.client = AzureOpenAI(api_key=key, azure_endpoint=kwargs["base_url"], api_version="2024-02-01")
        self.model_name = model_name
        self.lang = lang


class XinferenceSeq2txt(Base):
    def __init__(self, key, model_name="whisper-small", **kwargs):
        self.base_url = kwargs.get('base_url', None)
        self.model_name = model_name
        self.key = key

    def transcription(self, audio, language="zh", prompt=None, response_format="json", temperature=0.7):
        if isinstance(audio, str):
            audio_file = open(audio, 'rb')
            audio_data = audio_file.read()
            audio_file_name = audio.split("/")[-1]
        else:
            audio_data = audio
            audio_file_name = "audio.wav"

        payload = {
            "model": self.model_name,
            "language": language,
            "prompt": prompt,
            "response_format": response_format,
            "temperature": temperature
        }

        files = {
            "file": (audio_file_name, audio_data, 'audio/wav')
        }

        try:
            response = requests.post(
                f"{self.base_url}/v1/audio/transcriptions",
                files=files,
                data=payload
            )
            response.raise_for_status()
            result = response.json()

            if 'text' in result:
                transcription_text = result['text'].strip()
                return transcription_text, num_tokens_from_string(transcription_text)
            else:
                return "**ERROR**: Failed to retrieve transcription.", 0

        except requests.exceptions.RequestException as e:
            return f"**ERROR**: {str(e)}", 0


class TencentCloudSeq2txt(Base):
    def __init__(
            self, key, model_name="16k_zh", base_url="https://asr.tencentcloudapi.com"
    ):
        from tencentcloud.common import credential
        from tencentcloud.asr.v20190614 import asr_client

        key = json.loads(key)
        sid = key.get("tencent_cloud_sid", "")
        sk = key.get("tencent_cloud_sk", "")
        cred = credential.Credential(sid, sk)
        self.client = asr_client.AsrClient(cred, "")
        self.model_name = model_name

    def transcription(self, audio, max_retries=60, retry_interval=5):
        from tencentcloud.common.exception.tencent_cloud_sdk_exception import (
            TencentCloudSDKException,
        )
        from tencentcloud.asr.v20190614 import models
        import time

        b64 = self.audio2base64(audio)
        try:
            # dispatch disk
            req = models.CreateRecTaskRequest()
            params = {
                "EngineModelType": self.model_name,
                "ChannelNum": 1,
                "ResTextFormat": 0,
                "SourceType": 1,
                "Data": b64,
            }
            req.from_json_string(json.dumps(params))
            resp = self.client.CreateRecTask(req)

            # loop query
            req = models.DescribeTaskStatusRequest()
            params = {"TaskId": resp.Data.TaskId}
            req.from_json_string(json.dumps(params))
            retries = 0
            while retries < max_retries:
                resp = self.client.DescribeTaskStatus(req)
                if resp.Data.StatusStr == "success":
                    text = re.sub(
                        r"\[\d+:\d+\.\d+,\d+:\d+\.\d+\]\s*", "", resp.Data.Result
                    ).strip()
                    return text, num_tokens_from_string(text)
                elif resp.Data.StatusStr == "failed":
                    return (
                        "**ERROR**: Failed to retrieve speech recognition results.",
                        0,
                    )
                else:
                    time.sleep(retry_interval)
                    retries += 1
            return "**ERROR**: Max retries exceeded. Task may still be processing.", 0

        except TencentCloudSDKException as e:
            return "**ERROR**: " + str(e), 0
        except Exception as e:
            return "**ERROR**: " + str(e), 0
