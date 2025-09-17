"""Utility components for file management, formatting, and caching."""

import json
import shutil
import tempfile
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Optional, Any
import gc
import torch

from .models import TranscriptionResult, TranscriptionSegment
from .logger import get_whisperx_logger

logger = get_whisperx_logger(__name__)


class FileManager:
    """Manages file operations and output generation."""
    
    def __init__(self, base_output_dir: str = "./outputs"):
        self.base_output_dir = Path(base_output_dir)
        self.base_output_dir.mkdir(exist_ok=True)
    
    def create_job_directory(self, filename: str, job_id: Optional[str] = None) -> Path:
        """Create a unique directory for processing job."""
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        stem = Path(filename).stem
        
        if job_id:
            dir_name = f"{stem}_{job_id}_{timestamp}"
        else:
            dir_name = f"{stem}_{timestamp}"
        
        job_dir = self.base_output_dir / dir_name
        job_dir.mkdir(exist_ok=True)
        
        # Create subdirectories
        (job_dir / "segments").mkdir(exist_ok=True)
        (job_dir / "metadata").mkdir(exist_ok=True)
        
        return job_dir
    
    def save_result(self, result: TranscriptionResult, job_dir: Path, 
                   formats: List[str] = None) -> Dict[str, Path]:
        """Save transcription result in multiple formats."""
        if formats is None:
            formats = ["json", "txt", "srt", "tsv"]
        
        base_name = result.file_path.stem if result.file_path else "transcript"
        saved_files = {}
        
        # Save JSON with full metadata
        if "json" in formats:
            json_path = job_dir / f"{base_name}.json"
            self._save_json(result, json_path)
            saved_files["json"] = json_path
        
        # Save plain text
        if "txt" in formats:
            txt_path = job_dir / f"{base_name}.txt"
            self._save_txt(result, txt_path)
            saved_files["txt"] = txt_path
        
        # Save SRT subtitles
        if "srt" in formats:
            srt_path = job_dir / f"{base_name}.srt"
            self._save_srt(result, srt_path)
            saved_files["srt"] = srt_path
        
        # Save TSV
        if "tsv" in formats:
            tsv_path = job_dir / f"{base_name}.tsv"
            self._save_tsv(result, tsv_path)
            saved_files["tsv"] = tsv_path
        
        # Save summary if available
        if result.summary:
            summary_path = job_dir / f"{base_name}_summary.txt"
            summary_path.write_text(result.summary, encoding="utf-8")
            saved_files["summary"] = summary_path
        
        # Save corrected text if available
        if result.corrected_text:
            corrected_path = job_dir / f"{base_name}_corrected.txt"
            corrected_path.write_text(result.corrected_text, encoding="utf-8")
            saved_files["corrected"] = corrected_path
        
        # Save metadata
        metadata_path = job_dir / "metadata" / "processing_info.json"
        self._save_metadata(result, metadata_path)
        saved_files["metadata"] = metadata_path
        
        return saved_files
    
    def _save_json(self, result: TranscriptionResult, path: Path):
        """Save result as JSON with full metadata."""
        data = {
            "segments": [
                {
                    "start": seg.start,
                    "end": seg.end,
                    "text": seg.text,
                    "speaker": seg.speaker,
                    "confidence": seg.confidence,
                    "words": seg.words
                }
                for seg in result.segments
            ],
            "language": result.language,
            "detected_language": result.detected_language,
            "duration": result.duration,
            "speakers": result.speakers,
            "corrected_text": result.corrected_text,
            "summary": result.summary,
            "processing_time": result.processing_time,
            "metadata": result.metadata or {}
        }
        
        with open(path, "w", encoding="utf-8") as f:
            json.dump(data, f, indent=2, ensure_ascii=False)
    
    def _save_txt(self, result: TranscriptionResult, path: Path):
        """Save result as plain text."""
        lines = []
        
        if result.speakers:
            for segment in result.segments:
                speaker = segment.speaker or "Unknown"
                lines.append(f"[{speaker}] {segment.text}")
        else:
            lines = [segment.text for segment in result.segments]
        
        path.write_text("\n".join(lines), encoding="utf-8")
    
    def _save_srt(self, result: TranscriptionResult, path: Path):
        """Save result as SRT subtitle format."""
        lines = []
        
        for i, segment in enumerate(result.segments, 1):
            start = segment.start
            end = segment.end
            text = segment.text
            
            # Format timestamps
            start_time = f"{int(start//3600):02d}:{int((start%3600)//60):02d}:{start%60:06.3f}".replace('.', ',')
            end_time = f"{int(end//3600):02d}:{int((end%3600)//60):02d}:{end%60:06.3f}".replace('.', ',')
            
            speaker = f"[{segment.speaker}] " if segment.speaker else ""
            lines.append(f"{i}\n{start_time} --> {end_time}\n{speaker}{text}\n")
        
        path.write_text("\n".join(lines), encoding="utf-8")
    
    def _save_tsv(self, result: TranscriptionResult, path: Path):
        """Save result as TSV format."""
        lines = ["start\tend\tspeaker\ttext"]
        
        for segment in result.segments:
            speaker = segment.speaker or ""
            lines.append(f"{segment.start}\t{segment.end}\t{speaker}\t{segment.text}")
        
        path.write_text("\n".join(lines), encoding="utf-8")
    
    def _save_metadata(self, result: TranscriptionResult, path: Path):
        """Save processing metadata."""
        metadata = {
            "processing_time": result.processing_time,
            "language": result.language,
            "detected_language": result.detected_language,
            "duration": result.duration,
            "speakers": result.speakers,
            "segment_count": len(result.segments),
            "has_correction": result.corrected_text is not None,
            "has_summary": result.summary is not None,
            "processed_at": datetime.now().isoformat()
        }
        
        path.parent.mkdir(exist_ok=True)
        with open(path, "w", encoding="utf-8") as f:
            json.dump(metadata, f, indent=2)
    
    def get_job_results(self) -> List[Dict[str, Any]]:
        """Get all processing results."""
        results = []
        
        for job_dir in self.base_output_dir.iterdir():
            if job_dir.is_dir():
                json_files = list(job_dir.glob("*.json"))
                if json_files:
                    try:
                        with open(json_files[0], "r", encoding="utf-8") as f:
                            data = json.load(f)
                        
                        results.append({
                            "folder": job_dir.name,
                            "audio_file": job_dir.name.split("_")[0],
                            "timestamp": "_".join(job_dir.name.split("_")[-2:]),
                            "path": job_dir,
                            "data": data,
                            "files": {
                                "json": job_dir / f"{job_dir.name.split('_')[0]}.json",
                                "txt": job_dir / f"{job_dir.name.split('_')[0]}.txt",
                                "srt": job_dir / f"{job_dir.name.split('_')[0]}.srt",
                                "tsv": job_dir / f"{job_dir.name.split('_')[0]}.tsv",
                            }
                        })
                    except Exception as e:
                        logger.warning(f"Error reading {json_files[0]}: {e}")
        
        return sorted(results, key=lambda x: x["timestamp"], reverse=True)


class OutputFormatter:
    """Handles output formatting and presentation."""
    
    @staticmethod
    def format_for_display(result: TranscriptionResult, 
                          include_timestamps: bool = True,
                          include_speakers: bool = True) -> str:
        """Format result for display."""
        lines = []
        
        if include_timestamps:
            for segment in result.segments:
                start = OutputFormatter._format_time(segment.start)
                end = OutputFormatter._format_time(segment.end)
                
                if include_speakers and segment.speaker:
                    lines.append(f"[{start} - {end}] [{segment.speaker}] {segment.text}")
                else:
                    lines.append(f"[{start} - {end}] {segment.text}")
        else:
            if include_speakers and result.speakers:
                for segment in result.segments:
                    speaker = segment.speaker or "Unknown"
                    lines.append(f"[{speaker}] {segment.text}")
            else:
                lines = [segment.text for segment in result.segments]
        
        return "\n".join(lines)
    
    @staticmethod
    def _format_time(seconds: float) -> str:
        """Format time in HH:MM:SS format."""
        hours = int(seconds // 3600)
        minutes = int((seconds % 3600) // 60)
        secs = int(seconds % 60)
        return f"{hours:02d}:{minutes:02d}:{secs:02d}"


class CacheManager:
    """Manages model caching and memory optimization."""
    
    def __init__(self):
        self._cache = {}
    
    def clear_cache(self, model_type: Optional[str] = None):
        """Clear model cache."""
        if model_type:
            keys_to_remove = [k for k in self._cache.keys() if k.startswith(model_type)]
            for key in keys_to_remove:
                del self._cache[key]
        else:
            self._cache.clear()
        
        # Clear GPU cache
        if torch.cuda.is_available():
            torch.cuda.empty_cache()
        
        # Force garbage collection
        gc.collect()
    
    def get_cache_stats(self) -> Dict[str, int]:
        """Get cache statistics."""
        stats = {}
        for key in self._cache.keys():
            model_type = key.split("_")[0]
            stats[model_type] = stats.get(model_type, 0) + 1
        return stats
    
    def get_memory_usage(self) -> Dict[str, float]:
        """Get current memory usage."""
        import psutil
        import os
        
        process = psutil.Process(os.getpid())
        ram_mb = process.memory_info().rss / 1024 / 1024
        
        gpu_mb = None
        if torch.cuda.is_available():
            gpu_mb = torch.cuda.memory_allocated() / 1024 / 1024
        
        return {
            "ram_mb": ram_mb,
            "gpu_mb": gpu_mb,
            "cached_models": len(self._cache)
        }