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

import logging
import time
import threading
from collections import deque
from dataclasses import dataclass, field
from typing import Dict, Optional, Any
import psutil
import json


@dataclass
class RaptorMetrics:
    """Comprehensive metrics for RAPTOR operations."""
    
    # Processing metrics
    total_documents_processed: int = 0
    total_chunks_processed: int = 0
    total_summaries_generated: int = 0
    total_processing_time: float = 0.0
    
    # Model usage metrics
    llm_calls_total: int = 0
    llm_calls_successful: int = 0
    llm_calls_failed: int = 0
    llm_total_tokens: int = 0
    llm_cache_hits: int = 0
    
    embedding_calls_total: int = 0
    embedding_calls_successful: int = 0
    embedding_calls_failed: int = 0
    embedding_cache_hits: int = 0
    
    # Clustering metrics
    clustering_operations: int = 0
    clustering_failures: int = 0
    average_clusters_per_layer: float = 0.0
    
    # Resource metrics
    peak_memory_mb: float = 0.0
    average_memory_mb: float = 0.0
    cpu_usage_percent: float = 0.0
    
    # Error metrics
    validation_errors: int = 0
    timeout_errors: int = 0
    resource_errors: int = 0
    other_errors: int = 0
    
    # Performance metrics
    average_processing_time_per_chunk: float = 0.0
    average_llm_response_time: float = 0.0
    average_embedding_time: float = 0.0
    
    # Recent performance history (last 100 operations)
    recent_processing_times: deque = field(default_factory=lambda: deque(maxlen=100))
    recent_memory_usage: deque = field(default_factory=lambda: deque(maxlen=100))
    
    def to_dict(self) -> Dict[str, Any]:
        """Convert metrics to dictionary for serialization."""
        return {
            'processing': {
                'total_documents_processed': self.total_documents_processed,
                'total_chunks_processed': self.total_chunks_processed,
                'total_summaries_generated': self.total_summaries_generated,
                'total_processing_time': self.total_processing_time,
                'average_processing_time_per_chunk': self.average_processing_time_per_chunk,
            },
            'llm': {
                'calls_total': self.llm_calls_total,
                'calls_successful': self.llm_calls_successful,
                'calls_failed': self.llm_calls_failed,
                'total_tokens': self.llm_total_tokens,
                'cache_hits': self.llm_cache_hits,
                'success_rate': self.llm_calls_successful / max(self.llm_calls_total, 1),
                'cache_hit_rate': self.llm_cache_hits / max(self.llm_calls_total, 1),
                'average_response_time': self.average_llm_response_time,
            },
            'embedding': {
                'calls_total': self.embedding_calls_total,
                'calls_successful': self.embedding_calls_successful,
                'calls_failed': self.embedding_calls_failed,
                'cache_hits': self.embedding_cache_hits,
                'success_rate': self.embedding_calls_successful / max(self.embedding_calls_total, 1),
                'cache_hit_rate': self.embedding_cache_hits / max(self.embedding_calls_total, 1),
                'average_response_time': self.average_embedding_time,
            },
            'clustering': {
                'operations': self.clustering_operations,
                'failures': self.clustering_failures,
                'success_rate': (self.clustering_operations - self.clustering_failures) / max(self.clustering_operations, 1),
                'average_clusters_per_layer': self.average_clusters_per_layer,
            },
            'resources': {
                'peak_memory_mb': self.peak_memory_mb,
                'average_memory_mb': self.average_memory_mb,
                'cpu_usage_percent': self.cpu_usage_percent,
            },
            'errors': {
                'validation_errors': self.validation_errors,
                'timeout_errors': self.timeout_errors,
                'resource_errors': self.resource_errors,
                'other_errors': self.other_errors,
                'total_errors': self.validation_errors + self.timeout_errors + self.resource_errors + self.other_errors,
            }
        }


class RaptorMonitor:
    """
    Comprehensive monitoring and metrics collection for RAPTOR operations.
    
    This class provides real-time monitoring, performance tracking, and
    alerting capabilities for RAPTOR processing.
    """
    
    def __init__(self, enable_detailed_logging: bool = True):
        self.metrics = RaptorMetrics()
        self.enable_detailed_logging = enable_detailed_logging
        self._lock = threading.Lock()
        self._process = psutil.Process()
        self._start_time = time.time()
        
        # Performance tracking
        self._operation_start_times = {}
        self._memory_samples = deque(maxlen=1000)
        
        # Alerting thresholds
        self.memory_alert_threshold_mb = 1024  # 1GB
        self.error_rate_alert_threshold = 0.1  # 10%
        self.response_time_alert_threshold = 30.0  # 30 seconds
        
        logging.info("RAPTOR Monitor initialized")
    
    def start_operation(self, operation_id: str, operation_type: str) -> None:
        """Start tracking an operation."""
        with self._lock:
            self._operation_start_times[operation_id] = {
                'start_time': time.time(),
                'type': operation_type
            }
    
    def end_operation(self, operation_id: str, success: bool = True, 
                     tokens_used: int = 0, cache_hit: bool = False) -> None:
        """End tracking an operation and update metrics."""
        with self._lock:
            if operation_id not in self._operation_start_times:
                return
            
            operation_info = self._operation_start_times.pop(operation_id)
            duration = time.time() - operation_info['start_time']
            operation_type = operation_info['type']
            
            # Update metrics based on operation type
            if operation_type == 'llm':
                self.metrics.llm_calls_total += 1
                if success:
                    self.metrics.llm_calls_successful += 1
                    self.metrics.llm_total_tokens += tokens_used
                else:
                    self.metrics.llm_calls_failed += 1
                
                if cache_hit:
                    self.metrics.llm_cache_hits += 1
                
                # Update average response time
                total_successful = self.metrics.llm_calls_successful
                if total_successful > 0:
                    self.metrics.average_llm_response_time = (
                        (self.metrics.average_llm_response_time * (total_successful - 1) + duration) / total_successful
                    )
            
            elif operation_type == 'embedding':
                self.metrics.embedding_calls_total += 1
                if success:
                    self.metrics.embedding_calls_successful += 1
                else:
                    self.metrics.embedding_calls_failed += 1
                
                if cache_hit:
                    self.metrics.embedding_cache_hits += 1
                
                # Update average response time
                total_successful = self.metrics.embedding_calls_successful
                if total_successful > 0:
                    self.metrics.average_embedding_time = (
                        (self.metrics.average_embedding_time * (total_successful - 1) + duration) / total_successful
                    )
            
            elif operation_type == 'clustering':
                self.metrics.clustering_operations += 1
                if not success:
                    self.metrics.clustering_failures += 1
            
            # Check for alerts
            self._check_alerts(operation_type, duration, success)
    
    def record_document_processed(self, chunks_count: int, summaries_count: int, 
                                processing_time: float) -> None:
        """Record completion of document processing."""
        with self._lock:
            self.metrics.total_documents_processed += 1
            self.metrics.total_chunks_processed += chunks_count
            self.metrics.total_summaries_generated += summaries_count
            self.metrics.total_processing_time += processing_time
            
            # Update recent performance
            self.metrics.recent_processing_times.append(processing_time)
            
            # Update average processing time per chunk
            if self.metrics.total_chunks_processed > 0:
                self.metrics.average_processing_time_per_chunk = (
                    self.metrics.total_processing_time / self.metrics.total_chunks_processed
                )
            
            # Sample memory usage
            self._sample_memory_usage()
    
    def record_error(self, error_type: str) -> None:
        """Record an error occurrence."""
        with self._lock:
            if error_type == 'validation':
                self.metrics.validation_errors += 1
            elif error_type == 'timeout':
                self.metrics.timeout_errors += 1
            elif error_type == 'resource':
                self.metrics.resource_errors += 1
            else:
                self.metrics.other_errors += 1
    
    def _sample_memory_usage(self) -> None:
        """Sample current memory usage."""
        try:
            memory_mb = self._process.memory_info().rss / 1024 / 1024
            self._memory_samples.append(memory_mb)
            self.metrics.recent_memory_usage.append(memory_mb)
            
            # Update peak and average memory
            self.metrics.peak_memory_mb = max(self.metrics.peak_memory_mb, memory_mb)
            if self._memory_samples:
                self.metrics.average_memory_mb = sum(self._memory_samples) / len(self._memory_samples)
            
            # Update CPU usage
            self.metrics.cpu_usage_percent = self._process.cpu_percent()
            
        except Exception as e:
            logging.warning(f"Failed to sample memory usage: {e}")
    
    def _check_alerts(self, operation_type: str, duration: float, success: bool) -> None:
        """Check for alert conditions."""
        # Memory alert
        current_memory = self._process.memory_info().rss / 1024 / 1024
        if current_memory > self.memory_alert_threshold_mb:
            logging.warning(f"RAPTOR memory usage alert: {current_memory:.1f}MB > {self.memory_alert_threshold_mb}MB")
        
        # Response time alert
        if duration > self.response_time_alert_threshold:
            logging.warning(f"RAPTOR slow operation alert: {operation_type} took {duration:.2f}s")
        
        # Error rate alert
        if operation_type == 'llm':
            error_rate = self.metrics.llm_calls_failed / max(self.metrics.llm_calls_total, 1)
            if error_rate > self.error_rate_alert_threshold:
                logging.warning(f"RAPTOR LLM error rate alert: {error_rate:.2%}")
        
        elif operation_type == 'embedding':
            error_rate = self.metrics.embedding_calls_failed / max(self.metrics.embedding_calls_total, 1)
            if error_rate > self.error_rate_alert_threshold:
                logging.warning(f"RAPTOR embedding error rate alert: {error_rate:.2%}")
    
    def get_metrics(self) -> Dict[str, Any]:
        """Get current metrics as dictionary."""
        with self._lock:
            return self.metrics.to_dict()
    
    def get_summary_report(self) -> str:
        """Get a human-readable summary report."""
        metrics_dict = self.get_metrics()
        uptime = time.time() - self._start_time
        
        report = f"""
RAPTOR Monitor Summary Report
============================
Uptime: {uptime:.1f} seconds

Processing:
- Documents processed: {metrics_dict['processing']['total_documents_processed']}
- Chunks processed: {metrics_dict['processing']['total_chunks_processed']}
- Summaries generated: {metrics_dict['processing']['total_summaries_generated']}
- Total processing time: {metrics_dict['processing']['total_processing_time']:.2f}s
- Avg time per chunk: {metrics_dict['processing']['average_processing_time_per_chunk']:.3f}s

LLM Performance:
- Total calls: {metrics_dict['llm']['calls_total']}
- Success rate: {metrics_dict['llm']['success_rate']:.2%}
- Cache hit rate: {metrics_dict['llm']['cache_hit_rate']:.2%}
- Avg response time: {metrics_dict['llm']['average_response_time']:.3f}s
- Total tokens: {metrics_dict['llm']['total_tokens']}

Embedding Performance:
- Total calls: {metrics_dict['embedding']['calls_total']}
- Success rate: {metrics_dict['embedding']['success_rate']:.2%}
- Cache hit rate: {metrics_dict['embedding']['cache_hit_rate']:.2%}
- Avg response time: {metrics_dict['embedding']['average_response_time']:.3f}s

Resources:
- Peak memory: {metrics_dict['resources']['peak_memory_mb']:.1f}MB
- Average memory: {metrics_dict['resources']['average_memory_mb']:.1f}MB
- CPU usage: {metrics_dict['resources']['cpu_usage_percent']:.1f}%

Errors:
- Total errors: {metrics_dict['errors']['total_errors']}
- Validation errors: {metrics_dict['errors']['validation_errors']}
- Timeout errors: {metrics_dict['errors']['timeout_errors']}
- Resource errors: {metrics_dict['errors']['resource_errors']}
"""
        return report
    
    def export_metrics(self, filepath: str) -> None:
        """Export metrics to JSON file."""
        try:
            metrics_dict = self.get_metrics()
            metrics_dict['export_timestamp'] = time.time()
            metrics_dict['uptime_seconds'] = time.time() - self._start_time
            
            with open(filepath, 'w') as f:
                json.dump(metrics_dict, f, indent=2)
            
            logging.info(f"RAPTOR metrics exported to {filepath}")
            
        except Exception as e:
            logging.error(f"Failed to export RAPTOR metrics: {e}")


# Global monitor instance
_global_monitor: Optional[RaptorMonitor] = None


def get_raptor_monitor() -> RaptorMonitor:
    """Get the global RAPTOR monitor instance."""
    global _global_monitor
    if _global_monitor is None:
        _global_monitor = RaptorMonitor()
    return _global_monitor


def initialize_raptor_monitor(enable_detailed_logging: bool = True) -> RaptorMonitor:
    """Initialize the global RAPTOR monitor."""
    global _global_monitor
    _global_monitor = RaptorMonitor(enable_detailed_logging)
    return _global_monitor
