#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
import logging
import re
from typing import Optional

from rag.nlp import rag_tokenizer, tokenize

logger = logging.getLogger("ragflow.video")

# ── tuneable constants ────────────────────────────────────────────────────────
SEGMENT_SECONDS: int = 60    # merge raw cues into ~60-second windows
OVERLAP_SECONDS: int = 10    # trailing overlap to avoid context cuts
# ─────────────────────────────────────────────────────────────────────────────


def _extract_video_id(url: str) -> Optional[str]:
    """Extract the 11-char YouTube video ID from any common URL format."""
    patterns = [
        r"(?:v=)([A-Za-z0-9_-]{11})",
        r"(?:youtu\.be/)([A-Za-z0-9_-]{11})",
        r"(?:embed/)([A-Za-z0-9_-]{11})",
    ]
    for pat in patterns:
        m = re.search(pat, url)
        if m:
            return m.group(1)
    return None


def _fetch_transcript(video_id: str, parser_config: dict | None = None) -> list:
    """
    Fetch transcript for a YouTube video using a configurable backend.

    Backend is selected via parser_config["whisper_backend"]:
      - "youtube-transcript-api"  : fast caption fetch, no download (default)
      - "faster-whisper"          : local Whisper via CTranslate2 (CPU/GPU)
      - "openai-whisper"          : local Whisper via original OpenAI lib (CPU/GPU)
      - "openai-api"              : cloud Whisper via OpenAI REST API (needs key)

    parser_config schema:
      {
        "whisper_backend": "faster-whisper",
        "whisper_model":   "base",
        "openai_api_key":  ""
      }

    Returns:
      [{"text": str, "start": float, "duration": float}, ...]
    """
    cfg = parser_config or {}
    backend = cfg.get("whisper_backend", "youtube-transcript-api")

    if backend == "youtube-transcript-api":
        return _fetch_transcript_yta(video_id)
    elif backend == "faster-whisper":
        return _fetch_transcript_faster_whisper(video_id, cfg)
    elif backend == "openai-whisper":
        return _fetch_transcript_openai_whisper(video_id, cfg)
    elif backend == "openai-api":
        return _fetch_transcript_openai_api(video_id, cfg)
    else:
        raise RuntimeError(
            f"Unknown whisper_backend '{backend}'. "
            "Choose: youtube-transcript-api | faster-whisper | openai-whisper | openai-api"
        )


# ---------------------------------------------------------------------------
# Backend 1: youtube-transcript-api (original, kept as fast local dev fallback)
# ---------------------------------------------------------------------------

def _fetch_transcript_yta(video_id: str) -> list:
    """Fetch captions via youtube-transcript-api. Fast, no audio download."""
    try:
        from youtube_transcript_api import YouTubeTranscriptApi
        from youtube_transcript_api._errors import (
            TranscriptsDisabled,
            NoTranscriptFound,
            VideoUnavailable,
        )
    except ImportError:
        raise RuntimeError(
            "youtube-transcript-api is not installed. "
            "Run: pip install youtube-transcript-api"
        )
    api = YouTubeTranscriptApi()
    try:
        fetched = api.fetch(video_id, languages=("en",))
        return fetched.to_raw_data()
    except Exception:
        pass
    try:
        transcript_list = api.list(video_id)
        try:
            transcript = transcript_list.find_manually_created_transcript(["en"])
        except NoTranscriptFound:
            try:
                transcript = transcript_list.find_generated_transcript(["en"])
            except NoTranscriptFound:
                available = [t.language_code for t in transcript_list]
                transcript = transcript_list.find_generated_transcript(available)
        fetched = api.fetch(video_id, languages=(transcript.language_code,))
        return fetched.to_raw_data()
    except TranscriptsDisabled:
        raise RuntimeError(f"Transcripts disabled for video {video_id}")
    except VideoUnavailable:
        raise RuntimeError(f"Video {video_id} is unavailable or private")
    except Exception as exc:
        raise RuntimeError(f"Failed to fetch transcript for {video_id}: {exc}") from exc


# ---------------------------------------------------------------------------
# Shared helpers for Whisper backends
# ---------------------------------------------------------------------------

def _download_audio(video_id: str) -> str:
    """
    Download YouTube audio to a temp .mp3 file using yt-dlp.
    Returns the path to the temp file. Caller is responsible for deletion.
    Requires ffmpeg to be installed in the container.
    """
    try:
        import yt_dlp
    except ImportError:
        raise RuntimeError("yt-dlp is not installed. Run: pip install yt-dlp")

    import os
    import tempfile

    tmp = tempfile.NamedTemporaryFile(suffix=".m4a", delete=False)
    tmp.close()
    os.unlink(tmp.name)  # remove so yt-dlp always downloads fresh
    out_path = tmp.name

    ydl_opts = {
        "format": "140/139/bestaudio[ext=m4a]/bestaudio",
        "outtmpl": out_path,
        "quiet": True,
        "no_warnings": True,
        "ffmpeg_location": "/usr/bin",
        "no_cache_dir": True,
    }

    url = f"https://www.youtube.com/watch?v={video_id}"
    with yt_dlp.YoutubeDL(ydl_opts) as ydl:
        ydl.download([url])

    candidate = out_path + ".m4a"
    if os.path.exists(candidate) and not os.path.exists(out_path):
        return candidate
    if os.path.exists(out_path):
        return out_path

    raise RuntimeError(f"yt-dlp did not produce expected audio file for video {video_id}")


def _segments_from_whisper_result(result: dict) -> list:
    """Convert openai-whisper result dict to standard format."""
    entries = []
    for seg in result.get("segments", []):
        start = float(seg.get("start", 0.0))
        end   = float(seg.get("end", start))
        entries.append({
            "text":     seg.get("text", "").strip(),
            "start":    round(start, 3),
            "duration": round(end - start, 3),
        })
    return entries


# ---------------------------------------------------------------------------
# Backend 2: faster-whisper (local, CTranslate2, CPU int8 or GPU float16)
# ---------------------------------------------------------------------------

def _fetch_transcript_faster_whisper(video_id: str, cfg: dict) -> list:
    """Transcribe via faster-whisper. Efficient on CPU with int8 quantisation."""
    try:
        from faster_whisper import WhisperModel
    except ImportError:
        raise RuntimeError(
            "faster-whisper is not installed. Run: pip install faster-whisper"
        )

    import os

    model_size = cfg.get("whisper_model", "base")
    device = cfg.get("whisper_device", "auto")

    if device == "auto":
        try:
            import torch
            device = "cuda" if torch.cuda.is_available() else "cpu"
        except ImportError:
            device = "cpu"

    compute_type = "float16" if device == "cuda" else "int8"

    audio_path = None
    try:
        audio_path = _download_audio(video_id)
        logger.info("video: faster-whisper transcribing %s (model=%s device=%s)",
                    video_id, model_size, device)
        model = WhisperModel(model_size, device=device, compute_type=compute_type)
        segments_iter, _ = model.transcribe(audio_path, beam_size=5)
        entries = []
        for seg in segments_iter:
            entries.append({
                "text":     seg.text.strip(),
                "start":    round(seg.start, 3),
                "duration": round(seg.end - seg.start, 3),
            })
        logger.info("video: faster-whisper produced %d segments for %s", len(entries), video_id)
        return entries
    except Exception as exc:
        raise RuntimeError(
            f"faster-whisper transcription failed for {video_id}: {exc}"
        ) from exc
    finally:
        if audio_path:
            import os as _os
            if _os.path.exists(audio_path):
                _os.remove(audio_path)


# ---------------------------------------------------------------------------
# Backend 3: openai-whisper (local, original OpenAI library)
# ---------------------------------------------------------------------------

def _fetch_transcript_openai_whisper(video_id: str, cfg: dict) -> list:
    """Transcribe via openai-whisper (original local library)."""
    try:
        import whisper
    except ImportError:
        raise RuntimeError(
            "openai-whisper is not installed. Run: pip install openai-whisper"
        )

    import os

    model_size = cfg.get("whisper_model", "base")
    audio_path = None
    try:
        audio_path = _download_audio(video_id)
        logger.info("video: openai-whisper transcribing %s (model=%s)", video_id, model_size)
        import ssl
        ssl._create_default_https_context = ssl._create_unverified_context
        model  = whisper.load_model(model_size)
        result = model.transcribe(audio_path)
        entries = _segments_from_whisper_result(result)
        logger.info("video: openai-whisper produced %d segments for %s", len(entries), video_id)
        return entries
    except Exception as exc:
        raise RuntimeError(
            f"openai-whisper transcription failed for {video_id}: {exc}"
        ) from exc
    finally:
        if audio_path and os.path.exists(audio_path):
            os.remove(audio_path)


# ---------------------------------------------------------------------------
# Backend 4: openai-api (cloud Whisper, verbose_json for segment timestamps)
# ---------------------------------------------------------------------------

def _fetch_transcript_openai_api(video_id: str, cfg: dict) -> list:
    """Transcribe via OpenAI Whisper API. Fastest option; requires API key."""
    try:
        from openai import OpenAI
    except ImportError:
        raise RuntimeError(
            "openai package is not installed. Run: pip install openai"
        )

    import os

    api_key = cfg.get("openai_api_key") or os.environ.get("OPENAI_API_KEY", "")
    if not api_key:
        raise RuntimeError(
            "openai-api backend requires 'openai_api_key' in parser_config "
            "or the OPENAI_API_KEY environment variable."
        )

    audio_path = None
    try:
        audio_path = _download_audio(video_id)
        logger.info("video: openai-api transcribing %s", video_id)
        client = OpenAI(api_key=api_key)
        with open(audio_path, "rb") as f:
            response = client.audio.transcriptions.create(
                model="whisper-1",
                file=f,
                response_format="verbose_json",
                timestamp_granularities=["segment"],
            )
        entries = []
        for seg in response.segments:
            entries.append({
                "text":     seg.text.strip(),
                "start":    round(seg.start, 3),
                "duration": round(seg.end - seg.start, 3),
            })
        logger.info("video: openai-api produced %d segments for %s", len(entries), video_id)
        return entries
    except Exception as exc:
        raise RuntimeError(
            f"OpenAI API transcription failed for {video_id}: {exc}"
        ) from exc
    finally:
        if audio_path:
            import os as _os
            if _os.path.exists(audio_path):
                _os.remove(audio_path)


def _merge_into_segments(entries: list) -> list:
    """
    Merge raw cue-level entries into overlapping ~60-second segments.
    Returns list of {"text", "start", "end", "timestamp_seconds"}.
    """
    if not entries:
        return []

    segments = []
    window_start = entries[0]["start"]
    window_texts = []
    window_end = window_start

    for entry in entries:
        cue_start: float = entry["start"]
        cue_text: str = entry["text"].replace("\n", " ").strip()
        cue_end: float = cue_start + entry.get("duration", 0.0)

        if cue_start - window_start >= SEGMENT_SECONDS and window_texts:
            segments.append({
                "text": " ".join(window_texts),
                "start": window_start,
                "end": window_end,
                "timestamp_seconds": int(window_start),
            })
            # trailing overlap: carry back OVERLAP_SECONDS of context
            overlap_texts = [
                e["text"].replace("\n", " ").strip()
                for e in entries
                if e["start"] >= (window_end - OVERLAP_SECONDS)
                and e["start"] < cue_start
            ]
            window_start = cue_start
            window_texts = overlap_texts + [cue_text]
            window_end = cue_end
        else:
            window_texts.append(cue_text)
            window_end = cue_end

    if window_texts:
        segments.append({
            "text": " ".join(window_texts),
            "start": window_start,
            "end": window_end,
            "timestamp_seconds": int(window_start),
        })

    return segments


def chunk(filename, binary=None, from_page=0, to_page=100_000,
          lang="English", callback=None, kb_id=None,
          parser_config=None, tenant_id=None, **kwargs):
    """
    Main entry point called by task_executor.build_chunks().

    `filename` carries the YouTube URL — RagFlow stores the document
    source path here, and for video documents we register the URL as
    the filename.

    Returns a list of chunk dicts. Each must contain `content_with_weight`
    (the text to embed). All other keys become stored metadata.
    """
    youtube_url: str = filename.strip()

    logger.info("video.chunk: starting ingestion for %s", youtube_url)

    if callback:
        callback(0.05, "Extracting video ID from URL")

    video_id = _extract_video_id(youtube_url)
    if not video_id:
        msg = f"Cannot extract video ID from URL: {youtube_url!r}"
        logger.error("video.chunk: %s", msg)
        if callback:
            callback(-1, msg)
        return []

    if callback:
        callback(0.1, f"Fetching transcript for video {video_id}")

    try:
        raw_entries = _fetch_transcript(video_id, parser_config=parser_config)
    except RuntimeError as exc:
        logger.error("video.chunk: %s", exc)
        if callback:
            callback(-1, str(exc))
        return []

    if callback:
        callback(0.4, f"Fetched {len(raw_entries)} transcript cues — merging into segments")

    segments = _merge_into_segments(raw_entries)

    if not segments:
        msg = "Transcript is empty — no chunks produced"
        logger.warning("video.chunk: %s for %s", msg, youtube_url)
        if callback:
            callback(-1, msg)
        return []

    # retrieve video title from parser_config if provided
    video_title = ""
    if parser_config and isinstance(parser_config, dict):
        video_title = parser_config.get("video_title", "")
    if not video_title:
        video_title = youtube_url

    chunks = []
    for seg in segments:
        deeplink = f"https://www.youtube.com/watch?v={video_id}&t={seg['timestamp_seconds']}s"
        d = {
            "docnm_kwd": filename,
            "title_tks": rag_tokenizer.tokenize(
                re.sub(r"\.[a-zA-Z]+$", "", filename)
            ),
        }
        d["title_sm_tks"] = rag_tokenizer.fine_grained_tokenize(d["title_tks"])
        tokenize(d, seg["text"], lang.lower() == "english")
        d["content_with_weight"] = seg["text"]
        # ── video-specific metadata ────────────────────────────────────────
        d["youtube_url"] = youtube_url
        d["video_id"] = video_id
        d["video_title"] = video_title
        d["timestamp_seconds"] = seg["timestamp_seconds"]
        d["transcript_segment"] = deeplink
        # video has no page geometry — omit positions entirely so
        # add_positions() is not called; timestamp_seconds serves the same role
        chunks.append(d)

    logger.info(
        "video.chunk: produced %d chunks for video_id=%s",
        len(chunks), video_id,
    )
    if callback:
        callback(0.9, f"Produced {len(chunks)} transcript chunks")

    return chunks