"""Core processing components for audio transcription."""

import asyncio
import json
import tempfile
import time
from pathlib import Path
from typing import Dict, List, Optional, Tuple, Any

import librosa
import numpy as np
import openai
import torch
import whisperx
from pyannote.audio import Pipeline

from .models import ProcessingConfig, TranscriptionResult, TranscriptionSegment, ProcessingStatus
from .utils import FileManager, CacheManager
from .logger import get_whisperx_logger

logger = get_whisperx_logger(__name__)


class AudioProcessor:
    """Handles audio preprocessing and format conversion."""
    
    @staticmethod
    def load_audio(file_path: str, target_sr: int = 16000) -> Tuple[np.ndarray, int]:
        """Load and preprocess audio file."""
        try:
            audio, sr = librosa.load(file_path, sr=target_sr)
            duration = len(audio) / sr
            logger.info(f"Loaded audio: {duration:.2f}s at {sr}Hz")
            return audio, sr
        except Exception as e:
            logger.error(f"Failed to load audio: {e}")
            raise
    
    @staticmethod
    def extract_audio_from_video(video_path: str, output_path: str) -> bool:
        """Extract audio from video file using ffmpeg."""
        import subprocess
        try:
            cmd = [
                'ffmpeg', '-i', video_path, '-vn', '-acodec', 'pcm_s16le',
                '-ar', '16000', '-ac', '1', '-y', output_path
            ]
            subprocess.run(cmd, check=True, capture_output=True)
            logger.info(f"Extracted audio from video: {video_path}")
            return True
        except subprocess.CalledProcessError as e:
            logger.error(f"Failed to extract audio: {e}")
            return False


class TranscriptionEngine:
    """Core transcription engine using WhisperX."""
    
    def __init__(self, cache_manager: CacheManager):
        self.cache_manager = cache_manager
        self._models = {}
    
    def load_transcription_model(self, model_name: str, device: str, 
                               compute_type: str = "int8", 
                               asr_options: Optional[Dict] = None) -> Any:
        """Load WhisperX transcription model."""
        cache_key = f"transcription_{model_name}_{device}_{compute_type}"
        
        if cache_key not in self._models:
            logger.info(f"🔄 Loading transcription model: {model_name} on {device} with {compute_type}")
            start_time = time.time()
            
            # Clear GPU memory before loading new model
            if device == "cuda" and torch.cuda.is_available():
                logger.info("🧹 Clearing GPU memory before model loading")
                torch.cuda.empty_cache()
                import gc
                gc.collect()
                logger.info(f"💾 GPU memory after cleanup: {torch.cuda.memory_allocated() / 1024 / 1024:.1f} MB")
            
            try:
                logger.info(f"📥 Starting model download/loading for {model_name}...")
                model = whisperx.load_model(
                    model_name, device, compute_type=compute_type,
                    asr_options=asr_options or {}
                )
                self._models[cache_key] = model
                load_time = time.time() - start_time
                logger.info(f"✅ Successfully loaded {model_name} in {load_time:.2f}s")
                
                if device == "cuda" and torch.cuda.is_available():
                    logger.info(f"💾 GPU memory after loading: {torch.cuda.memory_allocated() / 1024 / 1024:.1f} MB")
                    
            except RuntimeError as e:
                if "out of memory" in str(e).lower():
                    logger.error(f"💥 CUDA out of memory loading {model_name}. Falling back to CPU.")
                    logger.info(f"🔄 Retrying model load on CPU...")
                    # Fallback to CPU
                    model = whisperx.load_model(
                        model_name, "cpu", compute_type=compute_type,
                        asr_options=asr_options or {}
                    )
                    self._models[cache_key] = model
                    load_time = time.time() - start_time
                    logger.info(f"✅ Successfully loaded {model_name} on CPU fallback in {load_time:.2f}s")
                else:
                    logger.error(f"❌ Failed to load {model_name}: {str(e)}")
                    raise
        else:
            logger.info(f"♻️ Using cached transcription model: {model_name}")
        
        return self._models[cache_key]
    
    def load_alignment_model(self, language_code: str, device: str) -> Tuple[Any, Dict]:
        """Load alignment model for word-level timestamps."""
        cache_key = f"alignment_{language_code}_{device}"
        
        if cache_key not in self._models:
            logger.info(f"🔄 Loading alignment model for language: {language_code} on {device}")
            start_time = time.time()
            
            try:
                model_a, metadata = whisperx.load_align_model(
                    language_code=language_code, device=device
                )
                self._models[cache_key] = (model_a, metadata)
                load_time = time.time() - start_time
                logger.info(f"✅ Successfully loaded alignment model for {language_code} in {load_time:.2f}s")
            except Exception as e:
                logger.error(f"❌ Failed to load alignment model for {language_code}: {str(e)}")
                raise
        else:
            logger.info(f"♻️ Using cached alignment model for: {language_code}")
        
        return self._models[cache_key]
    
    def load_diarization_model(self, device: str = "auto", hf_token: Optional[str] = None) -> Any:
        """Load speaker diarization model."""
        cache_key = f"diarization_{device}"
        
        if cache_key not in self._models:
            logger.info(f"🔄 Loading diarization model on {device}")
            start_time = time.time()
            
            # Clear GPU memory before loading new model  
            if device == "cuda" and torch.cuda.is_available():
                logger.info("🧹 Clearing GPU memory before diarization model loading")
                torch.cuda.empty_cache()
                import gc
                gc.collect()
                logger.info(f"💾 GPU memory after cleanup: {torch.cuda.memory_allocated() / 1024 / 1024:.1f} MB")
            
            try:
                logger.info("📥 Starting diarization model download/loading...")
                # Log token usage for debugging
                if hf_token:
                    logger.info("🔑 Using provided Hugging Face token for pyannote authentication")
                else:
                    logger.info("⚠️  No HF token provided - using public access (may fail for private models)")
                
                # Final debug: log the exact token being passed to pyannote
                logger.info(f"🔍 load_diarization_model: About to call Pipeline.from_pretrained with use_auth_token='{hf_token[:20]}'")
                
                diarize_model = Pipeline.from_pretrained(
                    "pyannote/speaker-diarization-3.1",
                    use_auth_token=hf_token  # Use provided HF token instead of None
                )
                # Convert "auto" to actual device
                actual_device = "cuda" if device == "auto" and torch.cuda.is_available() else "cpu" if device == "auto" else device
                logger.info(f"🎯 Moving diarization model to {actual_device}")
                diarize_model.to(torch.device(actual_device))
                self._models[cache_key] = diarize_model
                load_time = time.time() - start_time
                logger.info(f"✅ Successfully loaded diarization model in {load_time:.2f}s")
                
                if actual_device == "cuda" and torch.cuda.is_available():
                    logger.info(f"💾 GPU memory after diarization loading: {torch.cuda.memory_allocated() / 1024 / 1024:.1f} MB")
                    
            except RuntimeError as e:
                if "out of memory" in str(e).lower():
                    logger.error("💥 CUDA out of memory loading diarization model. Falling back to CPU.")
                    logger.info("🔄 Retrying diarization model load on CPU...")
                    # Fallback to CPU
                    diarize_model = Pipeline.from_pretrained(
                        "pyannote/speaker-diarization-3.1",
                        use_auth_token=hf_token  # Use provided HF token instead of None
                    )
                    diarize_model.to(torch.device("cpu"))
                    self._models[cache_key] = diarize_model
                    load_time = time.time() - start_time
                    logger.info(f"✅ Successfully loaded diarization model on CPU fallback in {load_time:.2f}s")
                else:
                    logger.error(f"❌ Failed to load diarization model: {str(e)}")
                    raise
        else:
            logger.info("♻️ Using cached diarization model")
        
        return self._models[cache_key]
    
    def clear_model_cache(self, model_types: Optional[List[str]] = None) -> Dict[str, int]:
        """Clear specific model types from cache to free memory."""
        if model_types is None:
            model_types = ["transcription", "alignment", "diarization"]
        
        cleared_count = {}
        for model_type in model_types:
            count = 0
            keys_to_remove = [k for k in self._models.keys() if k.startswith(model_type)]
            for key in keys_to_remove:
                del self._models[key]
                count += 1
            cleared_count[model_type] = count
            
        # Force garbage collection
        import gc
        gc.collect()
        
        # Clear GPU cache if available
        if torch.cuda.is_available():
            torch.cuda.empty_cache()
            torch.cuda.synchronize()
        
        logger.info(f"🧹 Cleared model cache: {cleared_count}")
        return cleared_count
    
    def transcribe(self, audio_path: str, config: ProcessingConfig, hf_token: Optional[str] = None) -> Dict[str, Any]:
        """Perform transcription with WhisperX using hybrid processing."""
        logger.info(f"🎙️ Starting transcription for: {Path(audio_path).name}")
        total_start_time = time.time()
        logger.info(f"🔍 TranscriptionEngine.transcribe: received hf_token = '{hf_token[:20] + '...' if hf_token and len(hf_token) > 20 else hf_token}' (length: {len(hf_token) if hf_token else 0})")
        
        device_info = config.get_device_allocation_summary()
        logger.info(f"🔧 Device allocation: {device_info}")
        
        start_time = time.time()
        result = {"segments": [], "language": config.language}
        
        try:
            # Step 1: Load transcription model
            logger.info(f"📥 Loading transcription model: {config.model_name} on {config.transcription_device}")
            model = self.load_transcription_model(
                config.model_name, 
                config.transcription_device,
                config.compute_type,
                asr_options={
                    "initial_prompt": config.initial_prompt,
                    "condition_on_previous_text": config.condition_on_previous_text,
                    "beam_size": config.beam_size,
                    "best_of": config.best_of,
                    # "vad_onset": config.vad_onset,
                    # "vad_offset": config.vad_offset
                }
            )
            
            # Step 2: Initial transcription
            logger.info("🔊 Starting transcription...")
            logger.info(f"🌐 Using language: {config.language}")
            
            # DEBUG: Log the exact language parameter being passed to WhisperX
            actual_language = None if config.language == "auto" else config.language
            logger.info(f"🔍 DEBUG: WhisperX model.transcribe will receive language parameter: {actual_language}")
            logger.info(f"🔍 DEBUG: config.language = '{config.language}' (type: {type(config.language)})")
            
            transcription_start = time.time()
            
            audio = whisperx.load_audio(audio_path)
            whisper_result = model.transcribe(
                audio, 
                batch_size=16,
                language=actual_language
            )
            
            transcription_time = time.time() - transcription_start
            logger.info(f"✅ Transcription completed in {transcription_time:.2f}s")
            
            result.update(whisper_result)
            
            # Step 3: Align timestamps (on potentially different device)
            if config.alignment_device and result["segments"]:
                logger.info(f"⏱️ Aligning timestamps on {config.alignment_device}...")
                align_start = time.time()
                
                model_a, metadata = whisperx.load_align_model(
                    language_code=result["language"], 
                    device=config.alignment_device
                )
                result = whisperx.align(
                    result["segments"], 
                    model_a, 
                    metadata, 
                    audio, 
                    config.alignment_device,
                    return_char_alignments=False
                )
                
                align_time = time.time() - align_start
                logger.info(f"✅ Timestamp alignment completed in {align_time:.2f}s on {config.alignment_device}")
                
                # Clean up alignment model
                del model_a
                if config.alignment_device == "cuda":
                    torch.cuda.empty_cache()
            
            # Step 4: Speaker diarization (if enabled)
            if config.enable_diarization and result["segments"]:
                logger.info(f"👥 Starting speaker diarization on {config.diarization_device}...")
                diarize_start = time.time()
                logger.info(f"🔍 TranscriptionEngine.transcribe: Passing hf_token to load_diarization_model: '{hf_token[:20] + '...' if hf_token and len(hf_token) > 20 else hf_token}'")
                diarize_model = self.load_diarization_model(config.diarization_device, hf_token=hf_token)
                
                # Configure batch sizes for optimization
                if config.optimize_diarization:
                    logger.info(f"⚡ Optimizing diarization with batch size: {config.diarization_batch_size}")
                    if hasattr(diarize_model, '_segmentation'):
                        diarize_model._segmentation.batch_size = config.diarization_batch_size
                        logger.info("✅ Segmentation batch size configured")
                    if hasattr(diarize_model, '_embedding'):
                        diarize_model._embedding.batch_size = config.diarization_batch_size
                        logger.info("✅ Embedding batch size configured")
                
                logger.info(f"👥 Running diarization with {config.min_speakers}-{config.max_speakers} speakers...")
                diarize_segments = diarize_model(
                    {"waveform": torch.from_numpy(audio)[None, :], "sample_rate": 16000},
                    min_speakers=config.min_speakers,
                    max_speakers=config.max_speakers
                )
                
                # Format diarization results
                import pandas as pd
                
                def format_speaker_label(label):
                    """Format speaker label consistently."""
                    if '_' in label:
                        try:
                            # Try to extract number from label like "SPEAKER_00"
                            speaker_num = int(label.split("_")[-1])
                            return f'SPEAKER_{speaker_num:02d}'
                        except (ValueError, IndexError):
                            # If extraction fails, use label as-is
                            return label
                    else:
                        return label
                
                diarize_df = pd.DataFrame([{
                    'start': segment.start,
                    'end': segment.end,
                    'speaker': format_speaker_label(label)
                } for segment, _, label in diarize_segments.itertracks(yield_label=True)])
                
                logger.info(f"👥 Found {len(diarize_df)} diarization segments")
                unique_speakers = diarize_df['speaker'].unique()
                logger.info(f"👥 Detected speakers: {list(unique_speakers)}")
                
                result = whisperx.assign_word_speakers(diarize_df, result)
                diarize_time = time.time() - diarize_start
                logger.info(f"✅ Speaker diarization completed in {diarize_time:.2f}s")
            
            total_time = time.time() - total_start_time
            logger.info(f"🎉 Complete transcription pipeline finished in {total_time:.2f}s")
            
            # Log final statistics with hybrid processing info
            segments_count = len(result.get("segments", []))
            logger.info(f"📊 Final result: {segments_count} segments")
            
            # Log hybrid processing summary
            if config.enable_hybrid_processing:
                logger.info("🚀 Hybrid Processing Summary:")
                logger.info(f"  ✅ Transcription: {config.transcription_device}")
                logger.info(f"  ✅ Alignment: {config.alignment_device} (CPU offload saves GPU memory)")
                logger.info(f"  ✅ Diarization: {config.diarization_device} (GPU accelerated)")
                logger.info(f"  ✅ Memory optimization: {'enabled' if config.memory_optimization else 'disabled'}")
            
            return result
            
        except Exception as e:
            logger.error(f"❌ Transcription failed: {str(e)}")
            import traceback
            logger.error(f"🔍 Error details: {traceback.format_exc()}")
            raise


class LLMProcessor:
    """Handles LLM-based text correction and summarization."""
    
    def __init__(self, api_key: Optional[str] = None):
        if api_key:
            openai.api_key = api_key
        self.correction_prompt = """You are a professional transcription editor. 
        Correct the following transcribed text by:
        1. Fixing typos and transcription errors
        2. Adding proper punctuation and capitalization
        3. Improving readability while maintaining accuracy
        4. Preserving the original meaning and speaker attribution
        
        Return only the corrected text."""
        
        self.summarization_prompt = """You are a professional summarizer. 
        Create a concise summary of the following transcript that:
        1. Captures the main points and key information
        2. Maintains chronological order
        3. Uses clear, professional language
        4. Is approximately 10-20% of the original length
        
        Transcript:
        {text}
        
        Summary:"""
    
    def correct_text(self, text: str, language: str = "auto") -> str:
        """Apply LLM-based text correction."""
        try:
            if len(text.strip()) < 10:
                return text
            
            logger.info("🔄 Starting LLM text correction...")
            correction_start = time.time()
            
            language_context = f"Language: {language}" if language != "auto" else ""
            
            response = openai.chat.completions.create(
                model="gpt-3.5-turbo",
                temperature=0.3,
                messages=[
                    {
                        "role": "system",
                        "content": f"{self.correction_prompt}\n{language_context}"
                    },
                    {
                        "role": "user",
                        "content": text
                    }
                ]
            )
            
            corrected = response.choices[0].message.content.strip()
            correction_time = time.time() - correction_start
            logger.info(f"✅ LLM correction completed in {correction_time:.2f}s: {len(text)} -> {len(corrected)} chars")
            return corrected
            
        except Exception as e:
            logger.error(f"❌ LLM correction failed: {e}")
            return text
    
    def generate_summary(self, text: str, max_length: Optional[int] = None) -> str:
        """Generate summary using LLM."""
        try:
            if len(text.strip()) < 50:
                return "Text too short for meaningful summary."
            
            logger.info("🔄 Starting LLM summarization...")
            summary_start = time.time()
            
            if max_length:
                target_ratio = max_length / len(text)
                self.summarization_prompt = self.summarization_prompt.replace(
                    "10-20%", f"{int(target_ratio*100)}%"
                )
            
            response = openai.chat.completions.create(
                model="gpt-3.5-turbo",
                temperature=0.5,
                messages=[
                    {
                        "role": "system",
                        "content": "You are a professional summarizer."
                    },
                    {
                        "role": "user",
                        "content": self.summarization_prompt.format(text=text)
                    }
                ]
            )
            
            summary = response.choices[0].message.content.strip()
            summary_time = time.time() - summary_start
            logger.info(f"✅ LLM summarization completed in {summary_time:.2f}s: {len(text)} -> {len(summary)} chars")
            return summary
            
        except Exception as e:
            logger.error(f"❌ Summarization failed: {e}")
            return "Summary generation failed."


class ProcessingOrchestrator:
    """Orchestrates the complete transcription pipeline."""
    
    def __init__(self, cache_manager: CacheManager, api_key: Optional[str] = None, hf_token: Optional[str] = None):
        logger.info(f"🔍 ProcessingOrchestrator.__init__: hf_token = '{hf_token[:20] + '...' if hf_token and len(hf_token) > 20 else hf_token}' (length: {len(hf_token) if hf_token else 0})")
        
        self.audio_processor = AudioProcessor()
        self.transcription_engine = TranscriptionEngine(cache_manager)
        self.llm_processor = LLMProcessor(api_key)
        self.file_manager = FileManager()
        self.hf_token = hf_token  # Store HF token for pyannote authentication
        
        logger.info(f"🔍 ProcessingOrchestrator.__init__: Stored hf_token = '{self.hf_token[:20] + '...' if self.hf_token and len(self.hf_token) > 20 else self.hf_token}'")
    
    async def process_audio(self, file_path: str, config: ProcessingConfig, hf_token: Optional[str] = None) -> TranscriptionResult:
        """Process audio file through complete pipeline."""
        start_time = time.time()
        
        # Use provided hf_token or fall back to instance token
        auth_token = hf_token or self.hf_token
        logger.info(f"🔍 ProcessingOrchestrator.process_audio: provided hf_token = '{hf_token[:20] + '...' if hf_token and len(hf_token) > 20 else hf_token}'")
        logger.info(f"🔍 ProcessingOrchestrator.process_audio: instance hf_token = '{self.hf_token[:20] + '...' if self.hf_token and len(self.hf_token) > 20 else self.hf_token}'")
        logger.info(f"🔍 ProcessingOrchestrator.process_audio: final auth_token = '{auth_token[:20] + '...' if auth_token and len(auth_token) > 20 else auth_token}' (length: {len(auth_token) if auth_token else 0})")
        
        try:
            # Load audio
            audio, sr = self.audio_processor.load_audio(file_path)
            duration = len(audio) / sr
            
            # Transcribe (pass auth_token for pyannote authentication)
            logger.info(f"🔍 ProcessingOrchestrator.process_audio: Calling transcription_engine.transcribe with auth_token")
            whisper_result = self.transcription_engine.transcribe(file_path, config, hf_token=auth_token)
            
            # Convert to our format
            segments = []
            speakers = set()
            
            for segment in whisper_result.get("segments", []):
                transcription_segment = TranscriptionSegment(
                    start=segment["start"],
                    end=segment["end"],
                    text=segment["text"].strip(),
                    speaker=segment.get("speaker"),
                    words=segment.get("words", [])
                )
                segments.append(transcription_segment)
                
                if segment.get("speaker"):
                    speakers.add(segment["speaker"])
            
            # Get full text
            full_text = " ".join(seg.text for seg in segments)
            
            # Apply LLM corrections if enabled
            corrected_text = None
            if config.enable_llm_correction:
                logger.info("🤖 Starting LLM correction phase...")
                corrected_text = self.llm_processor.correct_text(
                    full_text, 
                    config.language
                )
            
            # Generate summary if enabled
            summary = None
            if config.enable_summarization:
                logger.info("📝 Starting LLM summarization phase...")
                summary = self.llm_processor.generate_summary(
                    corrected_text or full_text
                )
            
            # Create result
            result = TranscriptionResult(
                segments=segments,
                language=config.language,
                detected_language=whisper_result.get("language"),
                duration=duration,
                speakers=list(speakers) if speakers else None,
                corrected_text=corrected_text,
                summary=summary,
                processing_time=time.time() - start_time,
                file_path=Path(file_path)
            )
            
            logger.info(f"Processing completed in {result.processing_time:.2f}s")
            return result
            
        except Exception as e:
            logger.error(f"Processing failed: {e}")
            raise