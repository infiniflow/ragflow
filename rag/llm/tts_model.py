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

import _thread as thread
import base64
import hashlib
import hmac
import json
import queue
import re
import ssl
import time
from abc import ABC
from datetime import datetime
from time import mktime
from typing import Annotated, Literal
from urllib.parse import urlencode
from wsgiref.handlers import format_date_time

import httpx
import ormsgpack
import requests
import websocket
from pydantic import BaseModel, conint

from rag.utils import num_tokens_from_string


class ServeReferenceAudio(BaseModel):
    audio: bytes
    text: str


class ServeTTSRequest(BaseModel):
    text: str
    chunk_length: Annotated[int, conint(ge=100, le=300, strict=True)] = 200
    # Audio format
    format: Literal["wav", "pcm", "mp3"] = "mp3"
    mp3_bitrate: Literal[64, 128, 192] = 128
    # References audios for in-context learning
    references: list[ServeReferenceAudio] = []
    # Reference id
    # For example, if you want use https://fish.audio/m/7f92f8afb8ec43bf81429cc1c9199cb1/
    # Just pass 7f92f8afb8ec43bf81429cc1c9199cb1
    reference_id: str | None = None
    # Normalize text for en & zh, this increase stability for numbers
    normalize: bool = True
    # Balance mode will reduce latency to 300ms, but may decrease stability
    latency: Literal["normal", "balanced"] = "normal"


class Base(ABC):
    def __init__(self, key, model_name, base_url):
        pass

    def tts(self, audio):
        pass

    def normalize_text(self, text):
        return re.sub(r'(\*\*|##\d+\$\$|#)', '', text)


class FishAudioTTS(Base):
    def __init__(self, key, model_name, base_url="https://api.fish.audio/v1/tts"):
        if not base_url:
            base_url = "https://api.fish.audio/v1/tts"
        key = json.loads(key)
        self.headers = {
            "api-key": key.get("fish_audio_ak"),
            "content-type": "application/msgpack",
        }
        self.ref_id = key.get("fish_audio_refid")
        self.base_url = base_url

    def tts(self, text):
        from http import HTTPStatus

        text = self.normalize_text(text)
        request = ServeTTSRequest(text=text, reference_id=self.ref_id)

        with httpx.Client() as client:
            try:
                with client.stream(
                        method="POST",
                        url=self.base_url,
                        content=ormsgpack.packb(
                            request, option=ormsgpack.OPT_SERIALIZE_PYDANTIC
                        ),
                        headers=self.headers,
                        timeout=None,
                ) as response:
                    if response.status_code == HTTPStatus.OK:
                        for chunk in response.iter_bytes():
                            yield chunk
                    else:
                        response.raise_for_status()

                yield num_tokens_from_string(text)

            except httpx.HTTPStatusError as e:
                raise RuntimeError(f"**ERROR**: {e}")


class QwenTTS(Base):
    def __init__(self, key, model_name, base_url=""):
        import dashscope

        self.model_name = model_name
        dashscope.api_key = key

    def tts(self, text):
        from dashscope.api_entities.dashscope_response import SpeechSynthesisResponse
        from dashscope.audio.tts import ResultCallback, SpeechSynthesizer, SpeechSynthesisResult
        from collections import deque

        class Callback(ResultCallback):
            def __init__(self) -> None:
                self.dque = deque()

            def _run(self):
                while True:
                    if not self.dque:
                        time.sleep(0)
                        continue
                    val = self.dque.popleft()
                    if val:
                        yield val
                    else:
                        break

            def on_open(self):
                pass

            def on_complete(self):
                self.dque.append(None)

            def on_error(self, response: SpeechSynthesisResponse):
                raise RuntimeError(str(response))

            def on_close(self):
                pass

            def on_event(self, result: SpeechSynthesisResult):
                if result.get_audio_frame() is not None:
                    self.dque.append(result.get_audio_frame())

        text = self.normalize_text(text)
        callback = Callback()
        SpeechSynthesizer.call(model=self.model_name,
                               text=text,
                               callback=callback,
                               format="mp3")
        try:
            for data in callback._run():
                yield data
            yield num_tokens_from_string(text)

        except Exception as e:
            raise RuntimeError(f"**ERROR**: {e}")


class OpenAITTS(Base):
    def __init__(self, key, model_name="tts-1", base_url="https://api.openai.com/v1"):
        if not base_url:
            base_url = "https://api.openai.com/v1"
        self.api_key = key
        self.model_name = model_name
        self.base_url = base_url
        self.headers = {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json"
        }

    def tts(self, text, voice="alloy"):
        text = self.normalize_text(text)
        payload = {
            "model": self.model_name,
            "voice": voice,
            "input": text
        }

        response = requests.post(f"{self.base_url}/audio/speech", headers=self.headers, json=payload, stream=True)

        if response.status_code != 200:
            raise Exception(f"**Error**: {response.status_code}, {response.text}")
        for chunk in response.iter_content():
            if chunk:
                yield chunk


class SparkTTS:
    STATUS_FIRST_FRAME = 0
    STATUS_CONTINUE_FRAME = 1
    STATUS_LAST_FRAME = 2

    def __init__(self, key, model_name, base_url=""):
        key = json.loads(key)
        self.APPID = key.get("spark_app_id", "xxxxxxx")
        self.APISecret = key.get("spark_api_secret", "xxxxxxx")
        self.APIKey = key.get("spark_api_key", "xxxxxx")
        self.model_name = model_name
        self.CommonArgs = {"app_id": self.APPID}
        self.audio_queue = queue.Queue()

    # 用来存储音频数据

    # 生成url
    def create_url(self):
        url = 'wss://tts-api.xfyun.cn/v2/tts'
        now = datetime.now()
        date = format_date_time(mktime(now.timetuple()))
        signature_origin = "host: " + "ws-api.xfyun.cn" + "\n"
        signature_origin += "date: " + date + "\n"
        signature_origin += "GET " + "/v2/tts " + "HTTP/1.1"
        signature_sha = hmac.new(self.APISecret.encode('utf-8'), signature_origin.encode('utf-8'),
                                 digestmod=hashlib.sha256).digest()
        signature_sha = base64.b64encode(signature_sha).decode(encoding='utf-8')
        authorization_origin = "api_key=\"%s\", algorithm=\"%s\", headers=\"%s\", signature=\"%s\"" % (
            self.APIKey, "hmac-sha256", "host date request-line", signature_sha)
        authorization = base64.b64encode(authorization_origin.encode('utf-8')).decode(encoding='utf-8')
        v = {
            "authorization": authorization,
            "date": date,
            "host": "ws-api.xfyun.cn"
        }
        url = url + '?' + urlencode(v)
        return url

    def tts(self, text):
        BusinessArgs = {"aue": "lame", "sfl": 1, "auf": "audio/L16;rate=16000", "vcn": self.model_name, "tte": "utf8"}
        Data = {"status": 2, "text": base64.b64encode(text.encode('utf-8')).decode('utf-8')}
        CommonArgs = {"app_id": self.APPID}
        audio_queue = self.audio_queue
        model_name = self.model_name

        class Callback:
            def __init__(self):
                self.audio_queue = audio_queue

            def on_message(self, ws, message):
                message = json.loads(message)
                code = message["code"]
                sid = message["sid"]
                audio = message["data"]["audio"]
                audio = base64.b64decode(audio)
                status = message["data"]["status"]
                if status == 2:
                    ws.close()
                if code != 0:
                    errMsg = message["message"]
                    raise Exception(f"sid:{sid} call error:{errMsg} code:{code}")
                else:
                    self.audio_queue.put(audio)

            def on_error(self, ws, error):
                raise Exception(error)

            def on_close(self, ws, close_status_code, close_msg):
                self.audio_queue.put(None)  # 放入 None 作为结束标志

            def on_open(self, ws):
                def run(*args):
                    d = {"common": CommonArgs,
                         "business": BusinessArgs,
                         "data": Data}
                    ws.send(json.dumps(d))

                thread.start_new_thread(run, ())

        wsUrl = self.create_url()
        websocket.enableTrace(False)
        a = Callback()
        ws = websocket.WebSocketApp(wsUrl, on_open=a.on_open, on_error=a.on_error, on_close=a.on_close,
                                    on_message=a.on_message)
        status_code = 0
        ws.run_forever(sslopt={"cert_reqs": ssl.CERT_NONE})
        while True:
            audio_chunk = self.audio_queue.get()
            if audio_chunk is None:
                if status_code == 0:
                    raise Exception(
                        f"Fail to access model({model_name}) using the provided credentials. **ERROR**: Invalid APPID, API Secret, or API Key.")
                else:
                    break
            status_code = 1
            yield audio_chunk


class XinferenceTTS:
    def __init__(self, key, model_name, **kwargs):
        self.base_url = kwargs.get("base_url", None)
        self.model_name = model_name
        self.headers = {
            "accept": "application/json",
            "Content-Type": "application/json"
        }

    def tts(self, text, voice="中文女", stream=True):
        payload = {
            "model": self.model_name,
            "input": text,
            "voice": voice
        }

        response = requests.post(
            f"{self.base_url}/v1/audio/speech",
            headers=self.headers,
            json=payload,
            stream=stream
        )

        if response.status_code != 200:
            raise Exception(f"**Error**: {response.status_code}, {response.text}")

        for chunk in response.iter_content(chunk_size=1024):
            if chunk:
                yield chunk


class OllamaTTS(Base):
    def __init__(self, key, model_name="ollama-tts", base_url="https://api.ollama.ai/v1"):
        if not base_url: 
            base_url = "https://api.ollama.ai/v1"
        self.model_name = model_name
        self.base_url = base_url
        self.headers = {
            "Content-Type": "application/json"
        }

    def tts(self, text, voice="standard-voice"):
        payload = {
            "model": self.model_name,
            "voice": voice,
            "input": text
        }

        response = requests.post(f"{self.base_url}/audio/tts", headers=self.headers, json=payload, stream=True)

        if response.status_code != 200:
            raise Exception(f"**Error**: {response.status_code}, {response.text}")

        for chunk in response.iter_content():
            if chunk:
                yield chunk

class GPUStackTTS:
    def __init__(self, key, model_name, **kwargs):
        self.base_url = kwargs.get("base_url", None)
        self.api_key = key
        self.model_name = model_name
        self.headers = {
            "accept": "application/json",
            "Content-Type": "application/json",
            "Authorization": f"Bearer {self.api_key}"
        }

    def tts(self, text, voice="Chinese Female", stream=True):
        payload = {
            "model": self.model_name,
            "input": text,
            "voice": voice
        }

        response = requests.post(
            f"{self.base_url}/v1-openai/audio/speech",
            headers=self.headers,
            json=payload,
            stream=stream
        )

        if response.status_code != 200:
            raise Exception(f"**Error**: {response.status_code}, {response.text}")

        for chunk in response.iter_content(chunk_size=1024):
            if chunk:
                yield chunk