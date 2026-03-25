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


def _fetch_transcript(video_id: str) -> list:
    """
    Fetch raw transcript entries from YouTube.
    Returns list of {"text": str, "start": float, "duration": float}.
    Raises RuntimeError on failure.

    Compatible with youtube-transcript-api >= 1.0.0 (instance-method API).
    Preference order: manual EN → auto-generated EN → first available language.
    """
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
        # fast path: directly fetch English transcript
        fetched = api.fetch(video_id, languages=("en",))
        return fetched.to_raw_data()
    except Exception:
        pass

    # fallback: inspect available transcripts and pick best option
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
        raw_entries = _fetch_transcript(video_id)
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
        # ── positional shim: map minute-bucket → fake page number ──────────
        # existing retrieval code reads `positions`; this keeps it working
        d["positions"] = [seg["timestamp_seconds"] // 60]
        chunks.append(d)

    logger.info(
        "video.chunk: produced %d chunks for video_id=%s",
        len(chunks), video_id,
    )
    if callback:
        callback(0.9, f"Produced {len(chunks)} transcript chunks")

    return chunks
