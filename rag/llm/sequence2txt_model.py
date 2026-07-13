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
import base64
import io
import json
import os
import re
from abc import ABC
import tempfile
import logging
from urllib.parse import urlparse

import requests
from openai import OpenAI
from openai.lib.azure import AzureOpenAI

from common.token_utils import num_tokens_from_string
from rag.utils.url_utils import ensure_v1


class Base(ABC):
    def __init__(self, key, model_name, **kwargs):
        """
        Abstract base class constructor.
        Parameters are not stored; initialization is left to subclasses.
        """
        pass

    def transcription(self, audio_path, **kwargs):
        with open(audio_path, "rb") as audio_file:
            transcription = self.client.audio.transcriptions.create(model=self.model_name, file=audio_file)
        return transcription.text.strip(), num_tokens_from_string(transcription.text.strip())

    def audio2base64(self, audio):
        if isinstance(audio, bytes):
            return base64.b64encode(audio).decode("utf-8")
        if isinstance(audio, io.BytesIO):
            return base64.b64encode(audio.getvalue()).decode("utf-8")
        raise TypeError("The input audio file should be in binary format.")


class GPTSeq2txt(Base):
    _FACTORY_NAME = "OpenAI"

    def __init__(self, key, model_name="whisper-1", base_url="https://api.openai.com/v1", **kwargs):
        if not base_url:
            base_url = "https://api.openai.com/v1"
        self.base_url = ensure_v1(base_url)
        self.client = OpenAI(api_key=key, base_url=self.base_url)
        self.model_name = model_name


class StepFunSeq2txt(GPTSeq2txt):
    _FACTORY_NAME = "StepFun"

    def __init__(self, key, model_name="step-asr", lang="Chinese", base_url="https://api.stepfun.com/v1", **kwargs):
        if not base_url:
            base_url = "https://api.stepfun.com/v1"
        super().__init__(key, model_name=model_name, base_url=base_url, **kwargs)


class FuturMixSeq2txt(GPTSeq2txt):
    _FACTORY_NAME = "FuturMix"

    def __init__(self, key, model_name="whisper-1", base_url="https://futurmix.ai/v1", **kwargs):
        if not base_url:
            base_url = "https://futurmix.ai/v1"
        super().__init__(key, model_name=model_name, base_url=base_url, **kwargs)
        logging.info("[FuturMix] Speech2Text initialized with model %s", model_name)


class QWenSeq2txt(Base):
    _FACTORY_NAME = "Tongyi-Qianwen"
    _FUN_ASR_FLASH_PREFIX = "fun-asr-flash"
    _FUN_ASR_BASE64_MAX_SIZE = 10 * 1024 * 1024
    _DASHSCOPE_API_BASE = "https://dashscope.aliyuncs.com/api/v1"
    _AUDIO_MIME_FORMATS = {
        "audio/mpeg": "mp3",
        "audio/mp3": "mp3",
        "audio/wav": "wav",
        "audio/wave": "wav",
        "audio/x-wav": "wav",
    }

    def __init__(self, key, model_name="qwen-audio-asr", base_url=None, **kwargs):
        import dashscope

        dashscope.api_key = key
        self.api_key = key
        self.model_name = model_name
        self.base_url = (base_url or self._DASHSCOPE_API_BASE).rstrip("/")

    def transcription(self, audio_path):
        # Fun-ASR-Flash uses DashScope's workspace-scoped native multimodal
        # endpoint and payload instead of MultiModalConversation.
        if self.model_name.startswith(self._FUN_ASR_FLASH_PREFIX):
            return self._transcribe_fun_asr_flash(audio_path)

        return self._transcribe_qwen_audio(audio_path)

    def _transcribe_qwen_audio(self, audio_path):
        import dashscope

        if audio_path.startswith("http"):
            audio_input = audio_path
        else:
            audio_input = f"file://{audio_path}"

        messages = [{"role": "system", "content": [{"text": ""}]}, {"role": "user", "content": [{"audio": audio_input}]}]

        resp = dashscope.MultiModalConversation.call(model=self.model_name, messages=messages, result_format="message", asr_options={"enable_lid": True, "enable_itn": False})

        try:
            text = resp["output"]["choices"][0]["message"].content[0]["text"]
        except Exception as e:
            text = "**ERROR**: " + str(e)
        return text, num_tokens_from_string(text)

    @classmethod
    def _fun_asr_audio_format(cls, audio_path):
        """Derive the Fun-ASR audio format from a data URI, URL, or path."""

        if audio_path.startswith("data:"):
            mime_type = audio_path[5:].split(";", 1)[0].lower()
            if not mime_type.startswith("audio/"):
                raise ValueError(f"Unsupported audio data URI MIME type: {mime_type or 'missing'}")
            audio_format = cls._AUDIO_MIME_FORMATS.get(mime_type, mime_type.split("/", 1)[1].removeprefix("x-"))
        else:
            path = urlparse(audio_path).path if audio_path.startswith(("http://", "https://")) else audio_path
            audio_format = os.path.splitext(path)[1].lower().lstrip(".")

        if audio_format == "wave":
            audio_format = "wav"
        if not audio_format:
            raise ValueError("Cannot determine audio format; use a URL/path extension or an audio data URI MIME type")
        return audio_format

    @classmethod
    def _validate_fun_asr_base64_size(cls, encoded_size):
        if encoded_size > cls._FUN_ASR_BASE64_MAX_SIZE:
            raise ValueError("Fun-ASR-Flash Base64 audio exceeds the 10 MB encoded-input limit; provide a publicly accessible URL (for example, OSS) instead")

    def _fun_asr_flash_request(self, audio_path, *, stream=False):
        audio_format = self._fun_asr_audio_format(audio_path)

        if audio_path.startswith(("http://", "https://")):
            audio_input = audio_path
        elif audio_path.startswith("data:"):
            _, separator, encoded_audio = audio_path.partition(",")
            if not separator:
                raise ValueError("Invalid audio data URI: missing Base64 payload")
            self._validate_fun_asr_base64_size(len(encoded_audio.encode("utf-8")))
            audio_input = audio_path
        else:
            file_size = os.path.getsize(audio_path)
            encoded_size = 4 * ((file_size + 2) // 3)
            self._validate_fun_asr_base64_size(encoded_size)
            mime_type = "audio/mpeg" if audio_format == "mp3" else f"audio/{audio_format}"
            with open(audio_path, "rb") as audio_file:
                audio_input = f"data:{mime_type};base64,{base64.b64encode(audio_file.read()).decode('utf-8')}"

        api_base = self.base_url
        if api_base.endswith("/compatible-mode/v1"):
            api_base = api_base[: -len("/compatible-mode/v1")] + "/api/v1"
        url = f"{api_base}/services/aigc/multimodal-generation/generation"
        payload = {
            "model": self.model_name,
            "input": {
                "messages": [
                    {
                        "role": "user",
                        "content": [{"type": "input_audio", "input_audio": {"data": audio_input}}],
                    }
                ]
            },
            # sample_rate is optional in the Fun-ASR-Flash API. Omitting it
            # avoids declaring incorrect metadata for remote or compressed audio.
            "parameters": {"format": audio_format},
        }
        headers = {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json",
            "X-DashScope-SSE": "enable" if stream else "disable",
        }
        return url, headers, payload

    def _transcribe_fun_asr_flash(self, audio_path):
        try:
            url, headers, payload = self._fun_asr_flash_request(audio_path)

            response = requests.post(url, headers=headers, json=payload, timeout=60)
            response.raise_for_status()
            result = response.json()
            text = result.get("text") or result.get("output", {}).get("text")
            if not text:
                raise ValueError("Missing transcription text in Fun-ASR-Flash response")
            text = text.strip()
            return text, num_tokens_from_string(text)
        except Exception as e:
            logging.exception("Fun-ASR-Flash transcription failed for model %s", self.model_name)
            return "**ERROR**: " + str(e), 0

    def _stream_fun_asr_flash(self, audio_path):
        try:
            url, headers, payload = self._fun_asr_flash_request(audio_path, stream=True)
            response = requests.post(url, headers=headers, json=payload, timeout=60, stream=True)
            response.raise_for_status()

            full = ""
            for line in response.iter_lines(decode_unicode=True):
                if not line or not line.startswith("data:"):
                    continue
                event_data = line[5:].strip()
                if not event_data or event_data == "[DONE]":
                    continue
                result = json.loads(event_data)
                text = result.get("text") or result.get("output", {}).get("text")
                if not text:
                    continue
                full = text.strip()
                yield {"event": "delta", "text": full}

            if not full:
                raise ValueError("Missing transcription text in Fun-ASR-Flash stream")
            yield {"event": "final", "text": full}
        except Exception as e:
            logging.exception("Fun-ASR-Flash streaming transcription failed for model %s", self.model_name)
            yield {"event": "error", "text": "**ERROR**: " + str(e)}

    def stream_transcription(self, audio_path):
        if self.model_name.startswith(self._FUN_ASR_FLASH_PREFIX):
            yield from self._stream_fun_asr_flash(audio_path)
            return

        import dashscope

        if audio_path.startswith("http"):
            audio_input = audio_path
        else:
            audio_input = f"file://{audio_path}"

        messages = [{"role": "system", "content": [{"text": ""}]}, {"role": "user", "content": [{"audio": audio_input}]}]

        stream = dashscope.MultiModalConversation.call(model=self.model_name, messages=messages, result_format="message", stream=True, asr_options={"enable_lid": True, "enable_itn": False})

        full = ""
        for chunk in stream:
            try:
                piece = chunk["output"]["choices"][0]["message"].content[0]["text"]
                full = piece
                yield {"event": "delta", "text": piece}
            except Exception as e:
                yield {"event": "error", "text": str(e)}

        yield {"event": "final", "text": full}


class AzureSeq2txt(Base):
    _FACTORY_NAME = "Azure-OpenAI"

    def __init__(self, key, model_name, lang="Chinese", **kwargs):
        self.base_url = ensure_v1(kwargs["base_url"])
        self.client = AzureOpenAI(api_key=key, azure_endpoint=self.base_url, api_version="2024-02-01")
        self.model_name = model_name
        self.lang = lang


class XinferenceSeq2txt(Base):
    _FACTORY_NAME = "Xinference"

    def __init__(self, key, model_name="whisper-small", **kwargs):
        self.base_url = ensure_v1(kwargs["base_url"]) if kwargs.get("base_url") else None
        self.model_name = model_name
        self.key = key

    def transcription(self, audio, language="zh", prompt=None, response_format="json", temperature=0.7):
        if isinstance(audio, str):
            with open(audio, "rb") as audio_file:
                audio_data = audio_file.read()
            audio_file_name = audio.split("/")[-1]
        else:
            audio_data = audio
            audio_file_name = "audio.wav"

        payload = {"model": self.model_name, "language": language, "prompt": prompt, "response_format": response_format, "temperature": temperature}

        files = {"file": (audio_file_name, audio_data, "audio/wav")}

        try:
            response = requests.post(f"{self.base_url}/v1/audio/transcriptions", files=files, data=payload, timeout=60)
            response.raise_for_status()
            result = response.json()

            if "text" in result:
                transcription_text = result["text"].strip()
                return transcription_text, num_tokens_from_string(transcription_text)
            else:
                return "**ERROR**: Failed to retrieve transcription.", 0

        except requests.exceptions.RequestException as e:
            return f"**ERROR**: {str(e)}", 0


class TencentCloudSeq2txt(Base):
    _FACTORY_NAME = "Tencent Cloud"

    def __init__(self, key, model_name="16k_zh", base_url="https://asr.tencentcloudapi.com"):
        from tencentcloud.asr.v20190614 import asr_client
        from tencentcloud.common import credential

        key = json.loads(key)
        sid = key.get("tencent_cloud_sid", "")
        sk = key.get("tencent_cloud_sk", "")
        cred = credential.Credential(sid, sk)
        self.client = asr_client.AsrClient(cred, "")
        self.model_name = model_name

    def transcription(self, audio, max_retries=60, retry_interval=5):
        import time

        from tencentcloud.asr.v20190614 import models
        from tencentcloud.common.exception.tencent_cloud_sdk_exception import (
            TencentCloudSDKException,
        )

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
                    text = re.sub(r"\[\d+:\d+\.\d+,\d+:\d+\.\d+\]\s*", "", resp.Data.Result).strip()
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


class GPUStackSeq2txt(Base):
    _FACTORY_NAME = "GPUStack"

    def __init__(self, key, model_name, base_url):
        if not base_url:
            raise ValueError("url cannot be None")
        if base_url.split("/")[-1] != "v1":
            base_url = os.path.join(base_url, "v1")
        self.base_url = base_url
        self.model_name = model_name
        self.key = key


class GiteeSeq2txt(Base):
    _FACTORY_NAME = "GiteeAI"

    def __init__(self, key, model_name="whisper-1", base_url="https://ai.gitee.com/v1/", **kwargs):
        if not base_url:
            base_url = "https://ai.gitee.com/v1/"
        self.base_url = ensure_v1(base_url)
        self.client = OpenAI(api_key=key, base_url=self.base_url)
        self.model_name = model_name


class DeepInfraSeq2txt(Base):
    _FACTORY_NAME = "DeepInfra"

    def __init__(self, key, model_name, base_url="https://api.deepinfra.com/v1/openai", **kwargs):
        if not base_url:
            base_url = "https://api.deepinfra.com/v1/openai"
        self.base_url = ensure_v1(base_url)
        self.client = OpenAI(api_key=key, base_url=self.base_url)
        self.model_name = model_name


class CometAPISeq2txt(Base):
    _FACTORY_NAME = "CometAPI"

    def __init__(self, key, model_name="whisper-1", base_url="https://api.cometapi.com/v1", **kwargs):
        if not base_url:
            base_url = "https://api.cometapi.com/v1"
        self.base_url = ensure_v1(base_url)
        self.client = OpenAI(api_key=key, base_url=self.base_url)
        self.model_name = model_name


class DeerAPISeq2txt(Base):
    _FACTORY_NAME = "DeerAPI"

    def __init__(self, key, model_name="whisper-1", base_url="https://api.deerapi.com/v1", **kwargs):
        if not base_url:
            base_url = "https://api.deerapi.com/v1"
        self.base_url = ensure_v1(base_url)
        self.client = OpenAI(api_key=key, base_url=self.base_url)
        self.model_name = model_name


class ZhipuSeq2txt(Base):
    _FACTORY_NAME = "ZHIPU-AI"

    def __init__(self, key, model_name="glm-asr", base_url="https://open.bigmodel.cn/api/paas/v4", **kwargs):
        if not base_url:
            base_url = "https://open.bigmodel.cn/api/paas/v4"
        self.base_url = base_url
        self.api_key = key
        self.model_name = model_name
        self.gen_conf = kwargs.get("gen_conf", {})
        self.stream = kwargs.get("stream", False)

    def _convert_to_wav(self, input_path):
        ext = os.path.splitext(input_path)[1].lower()
        if ext in [".wav", ".mp3"]:
            return input_path
        fd, out_path = tempfile.mkstemp(suffix=".wav")
        os.close(fd)
        try:
            import ffmpeg
            import imageio_ffmpeg as ffmpeg_exe

            ffmpeg_path = ffmpeg_exe.get_ffmpeg_exe()
            (ffmpeg.input(input_path).output(out_path, ar=16000, ac=1).overwrite_output().run(cmd=ffmpeg_path, quiet=True))
            return out_path
        except Exception as e:
            raise RuntimeError(f"audio convert failed: {e}")

    def transcription(self, audio_path):
        payload = {
            "model": self.model_name,
            "temperature": str(self.gen_conf.get("temperature", 0.75)) or "0.75",
            "stream": self.stream,
        }

        headers = {"Authorization": f"Bearer {self.api_key}"}
        converted = self._convert_to_wav(audio_path)

        with open(converted, "rb") as audio_file:
            files = {"file": audio_file}

            try:
                response = requests.post(
                    url=f"{self.base_url}/audio/transcriptions",
                    data=payload,
                    files=files,
                    headers=headers,
                    timeout=60,
                )
                body = response.json()
                if response.status_code == 200:
                    full_content = body["text"]
                    return full_content, num_tokens_from_string(full_content)
                else:
                    error = body["error"]
                    return f"**ERROR**: code: {error['code']}, message: {error['message']}", 0
            except Exception as e:
                return "**ERROR**: " + str(e), 0


class RAGconSeq2txt(Base):
    """
    RAGcon Sequence2Text Provider - routes through LiteLLM proxy

    Speech-to-text models routed through LiteLLM.
    Default Base URL: https://connect.ragcon.com/v1
    """

    _FACTORY_NAME = "RAGcon"

    def __init__(self, key, model_name, base_url=None, lang="English", **kwargs):
        # Use provided base_url or fallback to default
        if not base_url:
            base_url = "https://connect.ragcon.com/v1"

        self.base_url = ensure_v1(base_url)
        self.model_name = model_name
        self.key = key
        self.lang = lang

        self.client = OpenAI(api_key=key, base_url=self.base_url)

    def transcription(self, audio_path, **kwargs):
        """
        Transcribe audio file using RAGcon's OpenAI-compatible API.
        Uses Whisper's automatic language detection for German and English audio.

        Args:
            audio_path: Path to the audio file
            **kwargs: Additional parameters (currently unused but maintained for compatibility)

        Returns:
            tuple: (transcribed_text, token_count)
        """
        with open(audio_path, "rb") as audio_file:
            # Call RAGcon API - Whisper will auto-detect language
            transcription = self.client.audio.transcriptions.create(model=self.model_name, file=audio_file)

        # Return text and token count
        text = transcription.text.strip()
        return text, num_tokens_from_string(text)


class NewAPISeq2txt(GPTSeq2txt):
    _FACTORY_NAME = "New API"

    def __init__(self, key, model_name="whisper-1", base_url="", **kwargs):
        if not base_url:
            raise ValueError("url cannot be None")
        model_name = model_name.split("___")[0]
        super().__init__(key, model_name=model_name, base_url=base_url, **kwargs)
