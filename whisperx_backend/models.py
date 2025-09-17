"""Data models for the transcription backend."""

from dataclasses import dataclass
from typing import Dict, List, Optional, Any
from enum import Enum
from pathlib import Path
import datetime


class ProcessingStatus(Enum):
    """Processing status enumeration."""
    PENDING = "pending"
    PROCESSING = "processing"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


@dataclass
class ProcessingConfig:
    """Configuration for audio processing."""
    language: str = "auto"
    model_name: str = "large-v3"
    device: str = "auto"
    enable_diarization: bool = True
    min_speakers: int = 1
    max_speakers: int = 5
    initial_prompt: Optional[str] = None
    condition_on_previous_text: bool = True
    diarization_batch_size: int = 16
    optimize_diarization: bool = True
    compute_type: str = "int8"
    enable_llm_correction: bool = True
    enable_summarization: bool = True
    output_formats: List[str] = None
    
    # ===== NEW: WhisperX ASR Options =====
    beam_size: int = 5
    best_of: int = 5
    vad_onset: float = 0.500
    vad_offset: float = 0.363
    
    # ===== NEW: Hybrid Processing Configuration =====
    transcription_device: Optional[str] = None  # Override device for transcription
    alignment_device: Optional[str] = None      # Override device for alignment (CPU recommended)
    diarization_device: Optional[str] = None    # Override device for diarization (GPU recommended)
    enable_hybrid_processing: bool = True       # Enable smart device allocation
    memory_optimization: bool = True            # Enable memory optimization strategies
    
    def __post_init__(self):
        if self.output_formats is None:
            self.output_formats = ["json", "txt", "srt", "tsv"]
        
        # ===== Smart Device Allocation =====
        if self.enable_hybrid_processing:
            # If no specific devices set, use smart defaults
            import torch
            has_cuda = torch.cuda.is_available()
            base_device = "cuda" if (self.device == "auto" and has_cuda) else ("cpu" if self.device == "auto" else self.device)
            
            if self.transcription_device is None:
                self.transcription_device = base_device  # Keep transcription on main device
            
            if self.alignment_device is None:
                # Alignment works well on CPU and saves GPU memory
                self.alignment_device = "cpu"
            
            if self.diarization_device is None:
                # Diarization benefits significantly from GPU
                self.diarization_device = base_device
        else:
            # Use same device for all if hybrid processing disabled
            if self.transcription_device is None:
                self.transcription_device = self.device
            if self.alignment_device is None:
                self.alignment_device = self.device
            if self.diarization_device is None:
                self.diarization_device = self.device
    
    def get_device_allocation_summary(self) -> Dict[str, Any]:
        """Get summary of device allocation for logging."""
        return {
            "transcription": self.transcription_device,
            "alignment": self.alignment_device, 
            "diarization": self.diarization_device,
            "hybrid_mode": self.enable_hybrid_processing,
            "memory_optimization": self.memory_optimization,
            "recommendation": "alignment on CPU, diarization on GPU for optimal performance"
        }


@dataclass
class TranscriptionSegment:
    """Individual transcription segment."""
    start: float
    end: float
    text: str
    speaker: Optional[str] = None
    confidence: Optional[float] = None
    words: Optional[List[Dict[str, Any]]] = None


@dataclass
class TranscriptionResult:
    """Complete transcription result."""
    segments: List[TranscriptionSegment]
    language: str
    detected_language: Optional[str] = None
    duration: Optional[float] = None
    speakers: Optional[List[str]] = None
    corrected_text: Optional[str] = None
    summary: Optional[str] = None
    metadata: Optional[Dict[str, Any]] = None
    processing_time: Optional[float] = None
    file_path: Optional[Path] = None
    
    def get_full_text(self, include_speakers: bool = True) -> str:
        """Get the full transcribed text."""
        if not self.segments:
            return ""
        
        if include_speakers and any(seg.speaker for seg in self.segments):
            return "\n".join(
                f"[{seg.speaker}] {seg.text}" if seg.speaker else seg.text
                for seg in self.segments
            )
        else:
            return " ".join(seg.text for seg in self.segments)


@dataclass
class ProcessingJob:
    """Represents a processing job."""
    job_id: str
    file_path: Path
    config: ProcessingConfig
    status: ProcessingStatus
    created_at: datetime.datetime
    started_at: Optional[datetime.datetime] = None
    completed_at: Optional[datetime.datetime] = None
    result: Optional[TranscriptionResult] = None
    error: Optional[str] = None
    progress: float = 0.0
    
    @property
    def duration(self) -> Optional[float]:
        """Get processing duration in seconds."""
        if self.started_at and self.completed_at:
            return (self.completed_at - self.started_at).total_seconds()
        return None