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

import base64
import hashlib
import uuid
import requests
import threading
import subprocess
import sys
import os
import logging

def get_uuid():
    return uuid.uuid1().hex


def download_img(url):
    if not url:
        return ""
    response = requests.get(url)
    return "data:" + \
        response.headers.get('Content-Type', 'image/jpg') + ";" + \
        "base64," + base64.b64encode(response.content).decode("utf-8")


def hash_str2int(line: str, mod: int = 10 ** 8) -> int:
    return int(hashlib.sha1(line.encode("utf-8")).hexdigest(), 16) % mod

def convert_bytes(size_in_bytes: int) -> str:
    """
    Format size in bytes.
    """
    if size_in_bytes == 0:
        return "0 B"

    units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
    i = 0
    size = float(size_in_bytes)

    while size >= 1024 and i < len(units) - 1:
        size /= 1024
        i += 1

    if i == 0 or size >= 100:
        return f"{size:.0f} {units[i]}"
    elif size >= 10:
        return f"{size:.1f} {units[i]}"
    else:
        return f"{size:.2f} {units[i]}"


def once(func):
    """
    A thread-safe decorator that ensures the decorated function runs exactly once,
    caching and returning its result for all subsequent calls. This prevents
    race conditions in multi-thread environments by using a lock to protect
    the execution state.

    Args:
        func (callable): The function to be executed only once.

    Returns:
        callable: A wrapper function that executes `func` on the first call
                  and returns the cached result thereafter.

    Example:
        @once
        def compute_expensive_value():
            print("Computing...")
            return 42

        # First call: executes and prints
        # Subsequent calls: return 42 without executing
    """
    executed = False
    result = None
    lock = threading.Lock()
    def wrapper(*args, **kwargs):
        nonlocal executed, result
        with lock:
            if not executed:
                executed = True
                result = func(*args, **kwargs)
        return result
    return wrapper

@once
def pip_install_torch():
    """
    Install PyTorch with CUDA support if DEVICE=gpu.
    
    Note: In Docker deployments, PyTorch is installed during container startup
    via entrypoint.sh. This function serves as a fallback for non-Docker environments.
    """
    device = os.getenv("DEVICE", "cpu")
    if device == "cpu":
        return
    
    # Check if torch is already installed with CUDA support
    try:
        import torch
        if torch.cuda.is_available():
            logging.info(f"PyTorch {torch.__version__} with CUDA support already available")
            return
    except ImportError:
        pass
    
    # Auto-detect CUDA version
    cuda_version = _detect_cuda_version()
    cuda_index_url = f"https://download.pytorch.org/whl/{cuda_version}"
    cn_mirror_url = f"https://mirrors.aliyun.com/pytorch-wheels/{cuda_version}"
    logging.info(f"Installing PyTorch with CUDA support ({cuda_version})")
    
    pkg_names = ["torch>=2.5.0,<3.0.0"]
    
    # Try Chinese mirror first (faster for users in China), then fallback to official
    for index_url in [cn_mirror_url, cuda_index_url]:
        try:
            # Try uv first (preferred in container environment)
            subprocess.check_call(
                ["uv", "pip", "install", *pkg_names, "--index-url", index_url]
            )
            return
        except (subprocess.CalledProcessError, FileNotFoundError):
            try:
                # Fallback to pip if uv is not available
                subprocess.check_call(
                    [sys.executable, "-m", "pip", "install", *pkg_names, "--index-url", index_url]
                )
                return
            except subprocess.CalledProcessError:
                continue
    
    logging.error("Failed to install PyTorch with CUDA support")


def _map_driver_to_cuda(major_version: int) -> str:
    """
    Map NVIDIA driver major version to CUDA version.
    Driver >= 550 -> cu124 (RTX 40/50 series)
    Driver >= 525 -> cu121 (RTX 30/40 series)
    Driver >= 450 -> cu118 (GTX 10/16/20 series)
    """
    if major_version >= 550:
        return "cu124"
    elif major_version >= 525:
        return "cu121"
    elif major_version >= 450:
        return "cu118"
    return "cu124"  # default for modern GPUs


def _detect_cuda_version():
    """
    Auto-detect CUDA version for PyTorch installation.
    Returns a PyTorch wheel tag like 'cu118', 'cu121', 'cu124'.
    """
    driver_version = None
    
    # Try nvidia-smi
    try:
        result = subprocess.run(
            ["nvidia-smi", "--query-gpu=driver_version", "--format=csv,noheader"],
            capture_output=True, text=True, timeout=10
        )
        if result.returncode == 0 and result.stdout.strip():
            driver_version = result.stdout.strip().split("\n")[0]
    except Exception:
        pass
    
    # Try /proc/driver/nvidia/version (Linux fallback)
    if not driver_version:
        try:
            with open("/proc/driver/nvidia/version", "r") as f:
                import re
                match = re.search(r"Kernel Module\s+(\d+\.\d+)", f.read())
                if match:
                    driver_version = match.group(1)
        except Exception:
            pass
    
    # Map driver version to CUDA version
    if driver_version:
        try:
            major = int(driver_version.split(".")[0])
            return _map_driver_to_cuda(major)
        except (ValueError, IndexError):
            pass
    
    return "cu124"  # Default for modern GPUs
