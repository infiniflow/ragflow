from __future__ import annotations

import logging
from logging.handlers import RotatingFileHandler
import sys
from pathlib import Path
import os


_MONKEYOCR_LOGGER_INITIALIZED = False


def _ensure_log_dir() -> Path:
    # project root: parent of this file's directory
    project_root = Path(__file__).resolve().parents[1]
    log_dir = project_root / 'logs' / 'monkeyocr'
    log_dir.mkdir(parents=True, exist_ok=True)
    return log_dir


def _build_file_handler() -> RotatingFileHandler:
    log_dir = _ensure_log_dir()
    log_path = log_dir / 'monkeydoc.log'
    handler = RotatingFileHandler(str(log_path), maxBytes=10 * 1024 * 1024, backupCount=5, encoding='utf-8')
    fmt = logging.Formatter('%(asctime)s | %(levelname)s | %(name)s | %(message)s')
    handler.setFormatter(fmt)
    handler.setLevel(os.getenv('MONKEYDOC_LOG_LEVEL', 'INFO'))
    return handler


def get_monkeyocr_logger(name: str) -> logging.Logger:
    """Return a logger configured to write into the monkeyocr file log system."""
    global _MONKEYOCR_LOGGER_INITIALIZED
    logger = logging.getLogger(name)
    logger.setLevel(os.getenv('MONKEYDOC_LOG_LEVEL', 'INFO'))
    # Initialize root once to avoid duplicate handlers
    if not _MONKEYOCR_LOGGER_INITIALIZED:
        root = logging.getLogger('monkeydoc')
        # File handler
        if not any(isinstance(h, RotatingFileHandler) for h in root.handlers):
            root.addHandler(_build_file_handler())
        # Console handler (stdout)
        if os.getenv('MONKEYDOC_LOG_TO_STDOUT', '1') != '0':
            if not any(isinstance(h, logging.StreamHandler) for h in root.handlers):
                stream = logging.StreamHandler(stream=sys.stdout)
                stream.setFormatter(logging.Formatter('%(asctime)s | %(levelname)s | %(name)s | %(message)s'))
                root.addHandler(stream)
        root.propagate = False
        _MONKEYOCR_LOGGER_INITIALIZED = True
    # Attach to component loggers as well if empty
    if not any(isinstance(h, RotatingFileHandler) for h in logger.handlers):
        logger.addHandler(_build_file_handler())
        # mirror console handler choice
        if os.getenv('MONKEYDOC_LOG_TO_STDOUT', '1') != '0':
            logger.addHandler(logging.StreamHandler(stream=sys.stdout))
        logger.propagate = False
    return logger


