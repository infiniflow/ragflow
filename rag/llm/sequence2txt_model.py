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
import os
import requests
from openai.lib.azure import AzureOpenAI
import io
from abc import ABC
from openai import OpenAI
import json
import base64
import re
import logging
from rag.utils import num_tokens_from_string

logger = logging.getLogger(__name__)


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
    def __init__(self, key, model_name="whisper-1", base_url="https://api.openai.com/v1", lang="en"):
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


class WhisperXSeq2txt(Base):
    def __init__(self, key="", model_name="whisperx", **kwargs):
        """
        Initialize WhisperX local ASR model.
        
        Args:
            key: Configuration stored as JSON string (for RAGFlow compatibility)
            model_name: WhisperX model name (default: "whisperx")
            **kwargs: Additional configuration options including WhisperX-specific settings
        """
        # Debug logging for HF token tracing
        
        logger.info(f"🔍 WhisperXSeq2txt.__init__: key='{key[:20] + '...' if key and len(key) > 20 else key}' (length: {len(key) if key else 0})")
        logger.info(f"🔍 WhisperXSeq2txt.__init__: kwargs keys={list(kwargs.keys())}")
        
        # Set the internal model name for WhisperX (can be overridden)
        self.whisperx_model = kwargs.get('whisperx_model', 'large-v3')
        self.model_name = model_name
        
        # Parse configuration from key if it's a JSON string (from RAGFlow UI)
        config = {}
        if key and key.strip():
            try:
                config = json.loads(key)
                logger.info(f"🔍 WhisperXSeq2txt.__init__: Parsed JSON config keys={list(config.keys())}")
            except (json.JSONDecodeError, TypeError):
                # If parsing fails, key might be a regular API key or empty
                logger.info(f"🔍 WhisperXSeq2txt.__init__: Key is not JSON, treating as raw HF token")
        
        # Extract WhisperX-specific configuration (prefer config from key, then kwargs, then defaults)
        self.enable_diarization = config.get('enable_diarization', kwargs.get('enable_diarization', True))
        self.min_speakers = config.get('min_speakers', kwargs.get('min_speakers', 1))
        self.max_speakers = config.get('max_speakers', kwargs.get('max_speakers', 5))
        self.initial_prompt = config.get('initial_prompt', kwargs.get('initial_prompt', None))
        self.condition_on_previous_text = config.get('condition_on_previous_text', kwargs.get('condition_on_previous_text', True))
        self.diarization_batch_size = config.get('diarization_batch_size', kwargs.get('diarization_batch_size', 16))
        
        # Ensure initial_prompt is None if empty string
        if not self.initial_prompt:
            self.initial_prompt = None
        
        # Store other configuration options INCLUDING the key parameter
        self.config_options = kwargs
        self.config_options['key'] = key  # Ensure key is stored in config_options
        
        logger.info(f"🔍 WhisperXSeq2txt.__init__: Final config_options keys={list(self.config_options.keys())}")
        if 'key' in self.config_options:
            logger.info(f"🔍 WhisperXSeq2txt.__init__: config_options['key'] = '{self.config_options['key'][:20] + '...' if self.config_options['key'] and len(self.config_options['key']) > 20 else self.config_options['key']}'")
        
        # Initialize WhisperX API (lazy loading)
        self._whisperx_api = None
    
    def _get_whisperx_api(self):
        """Lazy initialization of WhisperX API."""
        if self._whisperx_api is None:
            try:
                # Import the WhisperX backend using symlink path
                import sys
                from pathlib import Path
                
                # Add the voice transcription backend to Python path
                ragflow_root = Path(__file__).parent.parent.parent
                backend_path = ragflow_root / "whisperx_backend"
                
                # Check if backend path exists and add to Python path
                if not backend_path.exists():
                    raise ImportError(
                        f"WhisperX backend directory not found at: {backend_path}. "
                        "Please ensure the WhisperX pipeline is properly installed and "
                        "the 'whisperx_backend' directory exists in the RAGFlow root directory."
                    )
                
                # Add to Python path if not already there
                if str(ragflow_root) not in sys.path:
                    sys.path.insert(0, str(ragflow_root))
                
                # Import TranscriptionAPI after ensuring the backend path exists
                from whisperx_backend import TranscriptionAPI
                
                # Extract HF token from config_options if available
                hf_token = self.config_options.get('key') if hasattr(self, 'config_options') else None
                logger.info(f"🔍 _get_whisperx_api: Extracting HF token from config_options")
                logger.info(f"🔍 _get_whisperx_api: config_options available: {hasattr(self, 'config_options')}")
                if hasattr(self, 'config_options'):
                    logger.info(f"🔍 _get_whisperx_api: config_options keys: {list(self.config_options.keys())}")
                    logger.info(f"🔍 _get_whisperx_api: hf_token = '{hf_token[:20] + '...' if hf_token and len(hf_token) > 20 else hf_token}' (length: {len(hf_token) if hf_token else 0})")
                else:
                    logger.warning(f"🔍 _get_whisperx_api: config_options not available!")
                
                # Initialize with HF token for pyannote authentication
                logger.info(f"🔍 _get_whisperx_api: Initializing TranscriptionAPI with hf_token")
                self._whisperx_api = TranscriptionAPI(hf_token=hf_token)
                
            except ImportError as e:
                raise ImportError(
                    "WhisperX backend not available. Please ensure the WhisperX pipeline "
                    "is properly installed. The backend should be accessible via "
                    "the 'whisperx_backend' directory in the RAGFlow root directory. "
                    f"Error: {e}"
                )
            except Exception as e:
                raise RuntimeError(
                    f"Failed to initialize WhisperX TranscriptionAPI: {e}"
                )
        return self._whisperx_api
    
    def transcription(self, audio, **kwargs):
        """
        Transcribe audio using WhisperX pipeline.
        
        Args:
            audio: Audio data as bytes or BytesIO
            **kwargs: Additional transcription options
            
        Returns:
            tuple: (transcribed_text, token_count)
        """
        import tempfile
        import os
        from rag.utils import num_tokens_from_string
        
        try:
            # Get WhisperX API instance
            whisperx_api = self._get_whisperx_api()
            
            # Handle audio input - WhisperX expects file paths
            if isinstance(audio, bytes):
                # Create temporary file from bytes
                with tempfile.NamedTemporaryFile(suffix='.wav', delete=False) as temp_file:
                    temp_file.write(audio)
                    temp_file_path = temp_file.name
            elif hasattr(audio, 'read'):
                # Handle file-like objects (BytesIO, etc.)
                with tempfile.NamedTemporaryFile(suffix='.wav', delete=False) as temp_file:
                    temp_file.write(audio.read())
                    temp_file_path = temp_file.name
            else:
                raise TypeError("Audio input must be bytes or file-like object")
            
            try:
                # Prepare configuration for WhisperX using instance configuration
                
                # Map RAGFlow language names to WhisperX language codes
                lang_mapping = {
                    'English': 'en',
                    'Chinese': 'zh',
                    'chinese': 'zh',
                    'english': 'en',
                    'Japanese': 'ja',
                    'japanese': 'ja',
                    'Korean': 'ko',
                    'korean': 'ko',
                    'French': 'fr',
                    'french': 'fr',
                    'German': 'de',
                    'german': 'de',
                    'Spanish': 'es',
                    'spanish': 'es',
                    'Italian': 'it',
                    'italian': 'it',
                    'Portuguese': 'pt',
                    'portuguese': 'pt',
                    'Russian': 'ru',
                    'russian': 'ru'
                }
                
                # Determine language from multiple sources with priority order
                detected_language = 'auto'  # default
                if 'language' in kwargs:
                    detected_language = kwargs['language']
                elif 'lang' in self.config_options:
                    # Map RAGFlow language to WhisperX language code
                    ragflow_lang = self.config_options['lang']
                    detected_language = lang_mapping.get(ragflow_lang, ragflow_lang.lower())

                # Define comprehensive exclusion list for RAGFlow parameters
                ragflow_exclusions = {
                    # RAGFlow-specific parameters that are incompatible with ProcessingConfig
                    'lang',                    # RAGFlow uses 'lang', ProcessingConfig uses 'language'
                    'base_url',                # RAGFlow API endpoint parameter
                    'key',                     # RAGFlow API key parameter - converted to hf_token separately
                    'whisperx_model',          # Internal parameter, already handled
                    # WhisperX parameters already explicitly set above
                    'enable_diarization', 'min_speakers', 'max_speakers', 
                    'initial_prompt', 'condition_on_previous_text', 'diarization_batch_size',
                    # Additional RAGFlow/general parameters that might be passed
                    'api_key', 'api_base', 'tenant_id', 'llm_type', 'llm_name', 'llm_factory'
                }
                
                # Filter compatible parameters and log any excluded ones
                compatible_params = {}
                excluded_params = {}
                for k, v in self.config_options.items():
                    if k in ragflow_exclusions:
                        excluded_params[k] = v
                    else:
                        compatible_params[k] = v
                
                # Special handling for the API key - rename it to hf_token for the backend
                if 'key' in self.config_options and self.config_options['key']:
                    compatible_params['hf_token'] = self.config_options['key']
                    import logging
                    logger = logging.getLogger(__name__)
                    logger.debug(f"WhisperX: Converting RAGFlow API key to HF token for pyannote authentication")
                
                # Log parameter filtering for debugging (only if there are excluded params)
                if excluded_params:
                    import logging
                    logger = logging.getLogger(__name__)
                    logger.debug(f"WhisperX: Excluded RAGFlow parameters: {list(excluded_params.keys())}")
                    logger.debug(f"WhisperX: Using compatible parameters: {list(compatible_params.keys())}")
                
                transcription_config = {
                    'model_name': self.whisperx_model,  # Use the actual WhisperX model name
                    'language': detected_language,
                    'enable_diarization': self.enable_diarization,
                    'min_speakers': self.min_speakers,
                    'max_speakers': self.max_speakers,
                    'initial_prompt': self.initial_prompt,
                    'condition_on_previous_text': self.condition_on_previous_text,
                    'diarization_batch_size': self.diarization_batch_size,
                    'enable_llm_correction': kwargs.get('enable_llm_correction', False),  # Disabled by default for RAGFlow
                    'enable_summarization': False,  # Not needed for RAGFlow
                    'output_formats': ['json'],  # Only need structured output
                    # Add compatible parameters from config_options
                    **compatible_params
                }
                
                logger.info(f"🔍 DEBUG: Final transcription_config language = '{transcription_config.get('language')}'")
                logger.info(f"🔍 DEBUG: detected_language was set to = '{detected_language}'")
                logger.info(f"🔍 DEBUG: compatible_params = {compatible_params}")
                
                # FORCE the language to ensure it's not overridden by compatible_params
                transcription_config['language'] = detected_language  # Force language after compatible_params merge
                
                # Transcribe using WhisperX
                result = whisperx_api.transcribe_file(temp_file_path, **transcription_config)
                
                # Extract text from WhisperX result
                if result and result.segments:
                    # Get full text with speaker information if available
                    include_speakers = kwargs.get('include_speakers', True) and any(seg.speaker for seg in result.segments)
                    transcribed_text = result.get_full_text(include_speakers=include_speakers)
                else:
                    transcribed_text = ""
                
                if not transcribed_text.strip():
                    return "**ERROR**: No transcription generated", 0
                
                # Calculate token count
                token_count = num_tokens_from_string(transcribed_text)
                
                return transcribed_text.strip(), token_count
                
            finally:
                # Clean up temporary file
                try:
                    os.unlink(temp_file_path)
                except OSError:
                    pass  # File might have been deleted already
                    
        except Exception as e:
            error_msg = f"**ERROR**: WhisperX transcription failed: {str(e)}"
            return error_msg, 0


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


class GPUStackSeq2txt(Base):
    def __init__(self, key, model_name, base_url):
        if not base_url:
            raise ValueError("url cannot be None")
        if base_url.split("/")[-1] != "v1":
            base_url = os.path.join(base_url, "v1")
        self.base_url = base_url
        self.model_name = model_name
        self.key = key
