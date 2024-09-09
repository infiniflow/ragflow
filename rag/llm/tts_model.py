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

from typing import Annotated, Literal
from abc import ABC
import httpx
import ormsgpack
from pydantic import BaseModel, conint
from rag.utils import num_tokens_from_string
import json
import re
import time
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