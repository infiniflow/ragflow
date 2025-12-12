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
import json
import logging
import os
import platform
import re
import subprocess
import sys
import tempfile
import threading
import time
import zipfile
from io import BytesIO
from os import PathLike
from pathlib import Path
from queue import Empty, Queue
from typing import Any, Callable, Optional

import numpy as np
import pdfplumber
import requests
from PIL import Image
from strenum import StrEnum

from deepdoc.parser.pdf_parser import RAGFlowPdfParser

LOCK_KEY_pdfplumber = "global_shared_lock_pdfplumber"
if LOCK_KEY_pdfplumber not in sys.modules:
    sys.modules[LOCK_KEY_pdfplumber] = threading.Lock()


class MinerUContentType(StrEnum):
    IMAGE = "image"
    TABLE = "table"
    TEXT = "text"
    EQUATION = "equation"
    CODE = "code"
    LIST = "list"
    DISCARDED = "discarded"
    HEADER = "header"
    PAGE_NUMBER = "page_number"


class MinerUParser(RAGFlowPdfParser):
    def __init__(self, mineru_path: str = "mineru", mineru_api: str = "http://host.docker.internal:9987", mineru_server_url: str = ""):
        self.mineru_path = Path(mineru_path)
        self.mineru_api = mineru_api.rstrip("/")
        self.mineru_server_url = mineru_server_url.rstrip("/")
        self.using_api = False
        self.outlines = []
        self.logger = logging.getLogger(self.__class__.__name__)
        self._img_path_cache = {}  # line_tag -> img_path mapping for crop() lookup
        self._native_img_map = {}  # line_tag -> native mineru image (image/table/equation)

    def _extract_zip_no_root(self, zip_path, extract_to, root_dir):
        self.logger.info(f"[MinerU] Extract zip: zip_path={zip_path}, extract_to={extract_to}, root_hint={root_dir}")
        with zipfile.ZipFile(zip_path, "r") as zip_ref:
            if not root_dir:
                files = zip_ref.namelist()
                if files and files[0].endswith("/"):
                    root_dir = files[0]
                else:
                    root_dir = None

            if not root_dir or not root_dir.endswith("/"):
                self.logger.info(f"[MinerU] No root directory found, extracting all (root_hint={root_dir})")
                zip_ref.extractall(extract_to)
                return

            root_len = len(root_dir)
            for member in zip_ref.infolist():
                filename = member.filename
                if filename == root_dir:
                    self.logger.info("[MinerU] Ignore root folder...")
                    continue

                path = filename
                if path.startswith(root_dir):
                    path = path[root_len:]

                full_path = os.path.join(extract_to, path)
                if member.is_dir():
                    os.makedirs(full_path, exist_ok=True)
                else:
                    os.makedirs(os.path.dirname(full_path), exist_ok=True)
                    with open(full_path, "wb") as f:
                        f.write(zip_ref.read(filename))

    def _is_http_endpoint_valid(self, url, timeout=5):
        try:
            response = requests.head(url, timeout=timeout, allow_redirects=True)
            return response.status_code in [200, 301, 302, 307, 308]
        except Exception:
            return False

    def check_installation(self, backend: str = "pipeline", server_url: Optional[str] = None) -> tuple[bool, str]:
        reason = ""

        valid_backends = ["pipeline", "vlm-http-client", "vlm-transformers", "vlm-vllm-engine"]
        if backend not in valid_backends:
            reason = "[MinerU] Invalid backend '{backend}'. Valid backends are: {valid_backends}"
            self.logger.warning(reason)
            return False, reason

        subprocess_kwargs = {
            "capture_output": True,
            "text": True,
            "check": True,
            "encoding": "utf-8",
            "errors": "ignore",
        }

        if platform.system() == "Windows":
            subprocess_kwargs["creationflags"] = getattr(subprocess, "CREATE_NO_WINDOW", 0)

        if server_url is None:
            server_url = self.mineru_server_url

        if backend == "vlm-http-client" and server_url:
            try:
                server_accessible = self._is_http_endpoint_valid(server_url + "/openapi.json")
                self.logger.info(f"[MinerU] vlm-http-client server check: {server_accessible}")
                if server_accessible:
                    self.using_api = False  # We are using http client, not API
                    return True, reason
                else:
                    reason = f"[MinerU] vlm-http-client server not accessible: {server_url}"
                    self.logger.warning(f"[MinerU] vlm-http-client server not accessible: {server_url}")
                    return False, reason
            except Exception as e:
                self.logger.warning(f"[MinerU] vlm-http-client server check failed: {e}")
                try:
                    response = requests.get(server_url, timeout=5)
                    self.logger.info(f"[MinerU] vlm-http-client server connection check: success with status {response.status_code}")
                    self.using_api = False
                    return True, reason
                except Exception as e:
                    reason = f"[MinerU] vlm-http-client server connection check failed: {server_url}: {e}"
                    self.logger.warning(f"[MinerU] vlm-http-client server connection check failed: {server_url}: {e}")
                    return False, reason

        try:
            result = subprocess.run([str(self.mineru_path), "--version"], **subprocess_kwargs)
            version_info = result.stdout.strip()
            if version_info:
                self.logger.info(f"[MinerU] Detected version: {version_info}")
            else:
                self.logger.info("[MinerU] Detected MinerU, but version info is empty.")
            return True, reason
        except subprocess.CalledProcessError as e:
            self.logger.warning(f"[MinerU] Execution failed (exit code {e.returncode}).")
        except FileNotFoundError:
            self.logger.warning("[MinerU] MinerU not found. Please install it via: pip install -U 'mineru[core]'")
        except Exception as e:
            self.logger.error(f"[MinerU] Unexpected error during installation check: {e}")

        # If executable check fails, try API check
        try:
            if self.mineru_api:
                # check openapi.json
                openapi_exists = self._is_http_endpoint_valid(self.mineru_api + "/openapi.json")
                if not openapi_exists:
                    reason = "[MinerU] Failed to detect vaild MinerU API server"
                    return openapi_exists, reason
                self.logger.info(f"[MinerU] Detected {self.mineru_api}/openapi.json: {openapi_exists}")
                self.using_api = openapi_exists
                return openapi_exists, reason
            else:
                self.logger.info("[MinerU] api not exists.")
        except Exception as e:
            reason = f"[MinerU] Unexpected error during api check: {e}"
            self.logger.error(f"[MinerU] Unexpected error during api check: {e}")
        return False, reason

    def _run_mineru(
        self, input_path: Path, output_dir: Path, method: str = "auto", backend: str = "pipeline", lang: Optional[str] = None, server_url: Optional[str] = None, callback: Optional[Callable] = None
    ):
        if self.using_api:
            self._run_mineru_api(input_path, output_dir, method, backend, lang, callback)
        else:
            self._run_mineru_executable(input_path, output_dir, method, backend, lang, server_url, callback)

    def _run_mineru_api(self, input_path: Path, output_dir: Path, method: str = "auto", backend: str = "pipeline", lang: Optional[str] = None, callback: Optional[Callable] = None):
        output_zip_path = os.path.join(str(output_dir), "output.zip")

        pdf_file_path = str(input_path)

        if not os.path.exists(pdf_file_path):
            raise RuntimeError(f"[MinerU] PDF file not exists: {pdf_file_path}")

        pdf_file_name = Path(pdf_file_path).stem.strip()
        output_path = os.path.join(str(output_dir), pdf_file_name, method)
        os.makedirs(output_path, exist_ok=True)

        files = {"files": (pdf_file_name + ".pdf", open(pdf_file_path, "rb"), "application/pdf")}

        data = {
            "output_dir": "./output",
            "lang_list": lang,
            "backend": backend,
            "parse_method": method,
            "formula_enable": True,
            "table_enable": True,
            "server_url": None,
            "return_md": True,
            "return_middle_json": True,
            "return_model_output": True,
            "return_content_list": True,
            "return_images": True,
            "response_format_zip": True,
            "start_page_id": 0,
            "end_page_id": 99999,
        }

        headers = {"Accept": "application/json"}
        try:
            self.logger.info(f"[MinerU] invoke api: {self.mineru_api}/file_parse")
            if callback:
                callback(0.20, f"[MinerU] invoke api: {self.mineru_api}/file_parse")
            response = requests.post(url=f"{self.mineru_api}/file_parse", files=files, data=data, headers=headers, timeout=1800)

            response.raise_for_status()
            if response.headers.get("Content-Type") == "application/zip":
                self.logger.info(f"[MinerU] zip file returned, saving to {output_zip_path}...")

                if callback:
                    callback(0.30, f"[MinerU] zip file returned, saving to {output_zip_path}...")

                with open(output_zip_path, "wb") as f:
                    f.write(response.content)

                self.logger.info(f"[MinerU] Unzip to {output_path}...")
                self._extract_zip_no_root(output_zip_path, output_path, pdf_file_name + "/")

                if callback:
                    callback(0.40, f"[MinerU] Unzip to {output_path}...")
            else:
                self.logger.warning("[MinerU] not zip returned from api：%s " % response.headers.get("Content-Type"))
        except Exception as e:
            raise RuntimeError(f"[MinerU] api failed with exception {e}")
        self.logger.info("[MinerU] Api completed successfully.")

    def _run_mineru_executable(
        self, input_path: Path, output_dir: Path, method: str = "auto", backend: str = "pipeline", lang: Optional[str] = None, server_url: Optional[str] = None, callback: Optional[Callable] = None
    ):
        cmd = [str(self.mineru_path), "-p", str(input_path), "-o", str(output_dir), "-m", method]
        if backend:
            cmd.extend(["-b", backend])
        if lang:
            cmd.extend(["-l", lang])
        if server_url and backend == "vlm-http-client":
            cmd.extend(["-u", server_url])

        self.logger.info(f"[MinerU] Running command: {' '.join(cmd)}")

        subprocess_kwargs = {
            "stdout": subprocess.PIPE,
            "stderr": subprocess.PIPE,
            "text": True,
            "encoding": "utf-8",
            "errors": "ignore",
            "bufsize": 1,
        }

        if platform.system() == "Windows":
            subprocess_kwargs["creationflags"] = getattr(subprocess, "CREATE_NO_WINDOW", 0)

        process = subprocess.Popen(cmd, **subprocess_kwargs)
        stdout_queue, stderr_queue = Queue(), Queue()

        def enqueue_output(pipe, queue, prefix):
            for line in iter(pipe.readline, ""):
                if line.strip():
                    queue.put((prefix, line.strip()))
            pipe.close()

        threading.Thread(target=enqueue_output, args=(process.stdout, stdout_queue, "STDOUT"), daemon=True).start()
        threading.Thread(target=enqueue_output, args=(process.stderr, stderr_queue, "STDERR"), daemon=True).start()

        while process.poll() is None:
            for q in (stdout_queue, stderr_queue):
                try:
                    while True:
                        prefix, line = q.get_nowait()
                        if prefix == "STDOUT":
                            self.logger.info(f"[MinerU] {line}")
                        else:
                            self.logger.warning(f"[MinerU] {line}")
                except Empty:
                    pass
            time.sleep(0.1)

        return_code = process.wait()
        if return_code != 0:
            raise RuntimeError(f"[MinerU] Process failed with exit code {return_code}")
        self.logger.info("[MinerU] Command completed successfully.")

    def __images__(self, fnm, zoomin: int = 1, page_from=0, page_to=600, callback=None):
        self.page_from = page_from
        self.page_to = page_to
        try:
            with pdfplumber.open(fnm) if isinstance(fnm, (str, PathLike)) else pdfplumber.open(BytesIO(fnm)) as pdf:
                self.pdf = pdf
                self.page_images = [p.to_image(resolution=72 * zoomin, antialias=True).original for _, p in enumerate(self.pdf.pages[page_from:page_to])]
        except Exception as e:
            self.page_images = None
            self.total_page = 0
            self.logger.exception(e)

    def _line_tag(self, bx):
        pn = [bx["page_idx"] + 1]
        positions = bx.get("bbox", (0, 0, 0, 0))
        x0, top, x1, bott = positions

        if hasattr(self, "page_images") and self.page_images and len(self.page_images) > bx["page_idx"]:
            page_width, page_height = self.page_images[bx["page_idx"]].size
            x0 = (x0 / 1000.0) * page_width
            x1 = (x1 / 1000.0) * page_width
            top = (top / 1000.0) * page_height
            bott = (bott / 1000.0) * page_height

        return "@@{}\t{:.1f}\t{:.1f}\t{:.1f}\t{:.1f}##".format("-".join([str(p) for p in pn]), x0, x1, top, bott)

    def _raw_line_tag(self, bx):
        """生成原始归一化坐标(0-1000)的line_tag,用于缓存key匹配"""
        pn = bx.get("page_idx", 0) + 1
        bbox = bx.get("bbox", [0, 0, 0, 0])
        x0, y0, x1, y1 = bbox
        return "@@{}\t{:.1f}\t{:.1f}\t{:.1f}\t{:.1f}##".format(pn, x0, x1, y0, y1)

    def crop(self, text, ZM=1, need_position=False):
        """
        MinerU专用智能crop：
        1. 混合使用原生图（表格/图片）+ 兜底图（页宽条带）
        2. 拼接时去重（相同bbox的图只用一次）
        3. 阈值控制（最多10张，总高<2000px）
        4. 保持高清（不缩放）
        """
        # 从text中提取原始tags（保持1-based页码）
        original_tags = re.findall(r"@@[0-9-]+\t[0-9.\t]+##", text)
        poss = self.extract_positions(text)
        
        if not poss or not original_tags:
            if need_position:
                return None, None
            return
        
        # 确保tags和poss数量一致
        if len(original_tags) != len(poss):
            self.logger.warning(f"[MinerU] Tag count ({len(original_tags)}) != position count ({len(poss)}), using first {min(len(original_tags), len(poss))} items")
            min_len = min(len(original_tags), len(poss))
            original_tags = original_tags[:min_len]
            poss = poss[:min_len]
        
        # Step 1: 收集所有tag对应的图片
        images_to_stitch = []
        seen_tags = set()  # 用于去重
        
        for tag, pos in zip(original_tags, poss):
            pns, left, right, top, bottom = pos
            if not pns:
                continue
            
            # ✅ 去重：如果tag已处理过，跳过
            if tag in seen_tags:
                self.logger.debug(f"[MinerU] Skipping duplicate tag: {tag}")
                continue
            seen_tags.add(tag)
            
            # 优先级1: 查找MinerU原生图（表格/图片/公式）
            native_img_path = self._find_native_image_path(tag)
            if native_img_path:
                try:
                    # ✅ 使用页宽标准化版本（原生图保留入库MinIO）
                    img = self._normalize_native_image_width(native_img_path, tag)
                    images_to_stitch.append(("native", img, pos, tag))
                    self.logger.debug(f"[MinerU] Using normalized native image for tag: {tag}")
                    continue
                except Exception as e:
                    self.logger.debug(f"[MinerU] Failed to load native image {native_img_path}: {e}")
            
            # 优先级2: 查找兜底生成的页宽图（缓存）
            cache = getattr(self, "_img_path_cache", {})
            if tag in cache:
                try:
                    img = Image.open(cache[tag])
                    images_to_stitch.append(("cached", img, pos, tag))
                    self.logger.debug(f"[MinerU] Using cached fallback image for tag: {tag}")
                    continue
                except Exception as e:
                    self.logger.debug(f"[MinerU] Failed to load cached image: {e}")
            
            # 优先级3: 完整页兜底（如果page_images可用）
            if hasattr(self, "page_images") and self.page_images:
                page_idx = pns[0]  # pns[0]是0-based的页索引
                if 0 <= page_idx < len(self.page_images):
                    img = self.page_images[page_idx]
                    images_to_stitch.append(("fullpage", img, pos, tag))
                    self.logger.debug(f"[MinerU] Using full page fallback for tag: {tag}, page_idx={page_idx}")
        
        if not images_to_stitch:
            self.logger.warning("[MinerU] No images found for chunk")
            if need_position:
                return None, None
            return
        
        # ✅ 兜底图≤3张时，拼接完整页（去重）
        fallback_count = sum(1 for src, _, _, _ in images_to_stitch if src == "cached")
        if fallback_count <= 3 and fallback_count > 0:
            self.logger.debug(f"[MinerU] Fallback count = {fallback_count}, using full page strategy")
            return self._handle_low_fallback_count(poss, need_position)
        
        # Step 2: 智能拼接（带阈值控制）
        return self._smart_stitch_with_thresholds(images_to_stitch, need_position)
    
    def _find_native_image_path(self, tag):
        """查找MinerU原生图片路径（表格/图片/公式）"""
        # 需要在_read_output时建立 tag → native_img_path 的映射
        native_map = getattr(self, "_native_img_map", {})
        return native_map.get(tag)
    
    def _normalize_native_image_width(self, native_img_path, tag):
        """
        将Native图标准化为页宽版本（仅用于拼接）
        
        原理：根据tag中的bbox，从页面重新裁剪页宽条带
        - 横向：0 到 页宽
        - 纵向：bbox的y范围
        
        Args:
            native_img_path: MinerU原生图路径（保留入库MinIO）
            tag: 包含page_idx和bbox信息的tag字符串
            
        Returns:
            页宽标准化后的Image对象，失败则返回原生图
        """
        try:
            # 解析tag获取page_idx和bbox
            import re
            match = re.match(r"@@(\d+)\s+([\d.]+)\s+([\d.]+)\s+([\d.]+)\s+([\d.]+)##", tag)
            if not match:
                # 解析失败，返回原生图
                return Image.open(native_img_path)
            
            page_num, x0_str, x1_str, y0_str, y1_str = match.groups()
            page_idx = int(page_num) - 1  # 转为0-based
            bbox = [float(x0_str), float(y0_str), float(x1_str), float(y1_str)]
            
            # 检查page_images可用性
            if not hasattr(self, "page_images") or not self.page_images:
                return Image.open(native_img_path)
            
            if page_idx < 0 or page_idx >= len(self.page_images):
                return Image.open(native_img_path)
            
            # 获取页面图片
            page_img = self.page_images[page_idx]
            page_width, page_height = page_img.size
            
            # bbox转像素
            px0, py0, px1, py1 = self._bbox_to_pixels(bbox, (page_width, page_height))
            
            # 裁剪页宽条带（横向全宽，纵向bbox范围）
            crop_y0 = max(0, min(py0, page_height))
            crop_y1 = max(crop_y0 + 1, min(py1, page_height))
            
            if crop_y1 - crop_y0 < 2:
                # bbox无效，返回原生图
                return Image.open(native_img_path)
            
            page_width_img = page_img.crop((0, crop_y0, page_width, crop_y1))
            self.logger.debug(f"[MinerU] Normalized native image to page-width: {page_width}x{crop_y1-crop_y0}px")
            return page_width_img
            
        except Exception as e:
            self.logger.debug(f"[MinerU] Failed to normalize native image, using original: {e}")
            return Image.open(native_img_path)
    
    def _handle_low_fallback_count(self, poss, need_position):
        """
        兜底图≤3张时，拼接涉及页面截图（去重）
        
        策略：
        - 提取所有涉及页码
        - 去重并限制最多3页
        - 拼接这些完整页
        
        Args:
            poss: positions列表
            need_position: 是否需要返回positions
            
        Returns:
            拼接的完整页截图，或单页截图
        """
        if not hasattr(self, "page_images") or not self.page_images:
            if need_position:
                return None, None
            return
        
        # 提取所有涉及页码（0-based），去重并排序
        page_indices = sorted(set(
            pns[0] for pns, _, _, _, _ in poss 
            if pns and 0 <= pns[0] < len(self.page_images)
        ))
        
        # 限制最多3页
        page_indices = page_indices[:3]
        
        if not page_indices:
            if need_position:
                return None, None
            return
        
        self.logger.info(f"[MinerU] Low fallback count, stitching {len(page_indices)} page(s): {[idx+1 for idx in page_indices]}")
        
        # 单页直接返回
        if len(page_indices) == 1:
            page_img = self.page_images[page_indices[0]]
            if need_position:
                return page_img, [[page_indices[0], 0, page_img.width, 0, page_img.height]]
            return page_img
        
        # 多页垂直拼接
        page_imgs_with_meta = [
            ("fullpage", self.page_images[idx], ([idx], 0, 0, 0, 0), f"@@{idx+1}\t0\t0\t0\t0##")
            for idx in page_indices
        ]
        
        return self._stitch_images_vertically(page_imgs_with_meta, need_position, gap=10)
    
    
    def _smart_stitch_with_thresholds(self, images_with_metadata, need_position):
        """
        智能拼接：应用阈值控制
        
        Thresholds:
        - MAX_COUNT: 最多20张图
        - MAX_HEIGHT: 总高度不超过4000px
        
        Strategies:
        - 数量过多: 均匀采样到12张（保留首尾）
        - 高度过高: 截断到4000px
        - 不缩放图片（保持高清）
        """
        MAX_COUNT = 20
        SAMPLE_TARGET = 12  # 采样目标数量
        MAX_HEIGHT = 4000
        GAP = 6
        
        # 1. 数量控制：如果超过20张，均匀采样到12张
        if len(images_with_metadata) > MAX_COUNT:
            self.logger.info(f"[MinerU] Too many images ({len(images_with_metadata)}), sampling to {SAMPLE_TARGET}")
            images_with_metadata = self._sample_images_uniformly(images_with_metadata, SAMPLE_TARGET)
        
        # 2. 高度控制：累加到4000px为止
        trimmed_images = []
        current_height = 0
        
        for src, img, pos, tag in images_with_metadata:
            if current_height + img.height > MAX_HEIGHT:
                self.logger.info(f"[MinerU] Reached max height {MAX_HEIGHT}px at {len(trimmed_images)} images, stopping")
                break
            trimmed_images.append((src, img, pos, tag))
            current_height += img.height + GAP
        
        # 至少保留一张图
        if not trimmed_images and images_with_metadata:
            trimmed_images = [images_with_metadata[0]]
        
        # 3. 垂直拼接（不缩放）
        return self._stitch_images_vertically(trimmed_images, need_position, GAP)
    
    def _sample_images_uniformly(self, images, target_count):
        """均匀采样：保留首尾，均匀抽取中间"""
        if len(images) <= target_count:
            return images
        
        sampled = [images[0]]  # 首张
        step = len(images) / (target_count - 1)
        for i in range(1, target_count - 1):
            idx = int(i * step)
            sampled.append(images[idx])
        sampled.append(images[-1])  # 末张
        return sampled
    
    def _stitch_images_vertically(self, images_with_metadata, need_position, gap):
        """垂直拼接图片（不加补丁，不缩放）"""
        if not images_with_metadata:
            if need_position:
                return None, None
            return
        
        imgs = [img for _, img, _, _ in images_with_metadata]
        positions_list = [pos for _, _, pos, _ in images_with_metadata]
        
        # 计算画布尺寸
        total_height = sum(img.height for img in imgs) + gap * (len(imgs) - 1)
        max_width = max(img.width for img in imgs)
        
        # 创建画布
        pic = Image.new("RGB", (max_width, total_height), (245, 245, 245))
        
        # 逐张粘贴（垂直堆叠）
        current_y = 0
        positions = []
        
        for idx, (img, pos) in enumerate(zip(imgs, positions_list)):
            pic.paste(img, (0, current_y))
            
            # 提取position信息
            if pos and len(pos) >= 5:
                pns, left, right, top, bottom = pos
                if pns:
                    page_num = pns[0] + getattr(self, "page_from", 0)
                    positions.append((page_num, int(left), int(right), int(top), int(bottom)))
            
            current_y += img.height + gap
        
        if need_position:
            return pic, positions if positions else [(0, 0, max_width, 0, total_height)]
        return pic

    @staticmethod
    def extract_positions(txt: str):
        poss = []
        for tag in re.findall(r"@@[0-9-]+\t[0-9.\t]+##", txt):
            pn, left, right, top, bottom = tag.strip("#").strip("@").split("\t")
            left, right, top, bottom = float(left), float(right), float(top), float(bottom)
            poss.append(([int(p) - 1 for p in pn.split("-")], left, right, top, bottom))
        return poss

    def _bbox_to_pixels(self, bbox, page_size):
        x0, y0, x1, y1 = bbox
        pw, ph = page_size
        maxv = max(bbox)
        # 经验：MinerU bbox 常为 0~1000 归一化；否则认为已是像素
        if maxv <= 1.5:
            sx, sy = pw, ph
        elif maxv <= 1200:
            sx, sy = pw / 1000.0, ph / 1000.0
        else:
            sx, sy = 1.0, 1.0
        return (
            int(x0 * sx),
            int(y0 * sy),
            int(x1 * sx),
            int(y1 * sy),
        )

    def _generate_missing_images(self, outputs: list[dict[str, Any]], subdir: Path, file_stem: str):
        """生成兜底图：按页宽（横向全宽，纵向按bbox）"""
        if not getattr(self, "page_images", None):
            return
        if not subdir:
            return
        img_root = subdir / "generated_images"
        img_root.mkdir(parents=True, exist_ok=True)
        text_types = {"text", "list", "header", "code", MinerUContentType.TEXT, MinerUContentType.LIST, MinerUContentType.EQUATION, MinerUContentType.CODE}
        generated = 0
        for idx, item in enumerate(outputs):
            if item.get("type") not in text_types:
                continue
            if item.get("img_path"):
                continue
            
            bbox = item.get("bbox")
            if not bbox or len(bbox) != 4:
                continue
            
            page_idx = int(item.get("page_idx", 0))
            if page_idx < 0 or page_idx >= len(self.page_images):
                continue
                
            x0, y0, x1, y1 = self._bbox_to_pixels(bbox, self.page_images[page_idx].size)
            
            # 获取页面尺寸
            pw, ph = self.page_images[page_idx].size
            
            # ✅ 改为按页宽生成：横向=整页宽度，纵向=bbox范围
            # x坐标：0 到 页宽
            # y坐标：bbox的y0到y1（clamp到页面内）
            crop_x0 = 0
            crop_x1 = pw
            crop_y0 = max(0, min(y0, ph))
            crop_y1 = max(0, min(y1, ph))

            # guard invalid bbox
            if crop_y1 - crop_y0 < 2:
                continue
                
            try:
                # 裁剪页宽条带
                cropped = self.page_images[page_idx].crop((crop_x0, crop_y0, crop_x1, crop_y1))
                fname = f"{file_stem}_gen_{idx}.jpg"
                out_path = img_root / fname
                cropped.save(out_path, format="JPEG", quality=80)
                img_path_str = str(out_path.resolve())
                item["img_path"] = img_path_str
                
                # Cache for crop() lookup: use raw 0-1000 normalized tag for consistent matching
                raw_tag = self._raw_line_tag(item)
                self._img_path_cache[raw_tag] = img_path_str
                generated += 1
            except Exception as e:
                self.logger.debug(f"[MinerU] skip image gen idx={idx} page={page_idx}: {e}")
                continue
                
        if generated:
            self.logger.info(f"[MinerU] generated {generated} page-width fallback images, cached {len(self._img_path_cache)} tags")

    def _read_output(self, output_dir: Path, file_stem: str, method: str = "auto", backend: str = "pipeline") -> list[dict[str, Any]]:
        candidates = []
        seen = set()

        def add_candidate_path(p: Path):
            if p not in seen:
                seen.add(p)
                candidates.append(p)

        if backend.startswith("vlm-"):
            add_candidate_path(output_dir / file_stem / "vlm")
            if method:
                add_candidate_path(output_dir / file_stem / method)
            add_candidate_path(output_dir / file_stem / "auto")
        else:
            if method:
                add_candidate_path(output_dir / file_stem / method)
            add_candidate_path(output_dir / file_stem / "vlm")
            add_candidate_path(output_dir / file_stem / "auto")

        json_file = None
        subdir = None
        attempted = []

        # mirror MinerU's sanitize_filename to align ZIP naming
        def _sanitize_filename(name: str) -> str:
            sanitized = re.sub(r"[/\\\.]{2,}|[/\\]", "", name)
            sanitized = re.sub(r"[^\w.-]", "_", sanitized, flags=re.UNICODE)
            if sanitized.startswith("."):
                sanitized = "_" + sanitized[1:]
            return sanitized or "unnamed"

        safe_stem = _sanitize_filename(file_stem)
        allowed_names = {f"{file_stem}_content_list.json", f"{safe_stem}_content_list.json"}
        self.logger.info(f"[MinerU] Expected output files: {', '.join(sorted(allowed_names))}")
        self.logger.info(f"[MinerU] Searching output candidates: {', '.join(str(c) for c in candidates)}")

        for sub in candidates:
            jf = sub / f"{file_stem}_content_list.json"
            self.logger.info(f"[MinerU] Trying original path: {jf}")
            attempted.append(jf)
            if jf.exists():
                subdir = sub
                json_file = jf
                break

            # MinerU API sanitizes non-ASCII filenames inside the ZIP root and file names.
            alt = sub / f"{safe_stem}_content_list.json"
            self.logger.info(f"[MinerU] Trying sanitized filename: {alt}")
            attempted.append(alt)
            if alt.exists():
                subdir = sub
                json_file = alt
                break

            nested_alt = sub / safe_stem / f"{safe_stem}_content_list.json"
            self.logger.info(f"[MinerU] Trying sanitized nested path: {nested_alt}")
            attempted.append(nested_alt)
            if nested_alt.exists():
                subdir = nested_alt.parent
                json_file = nested_alt
                break

        if not json_file:
            raise FileNotFoundError(f"[MinerU] Missing output file, tried: {', '.join(str(p) for p in attempted)}")

        with open(json_file, "r", encoding="utf-8") as f:
            data = json.load(f)

        # 建立 tag → 原生img_path 的映射（表格/图片/公式）
        self._native_img_map = {}
        
        for item in data:
            # 解析并补全路径
            for key in ("img_path", "table_img_path", "equation_img_path"):
                if key in item and item[key]:
                    item[key] = str((subdir / item[key]).resolve())
                    
                    # 建立映射: tag → native_img_path
                    try:
                        tag = self._raw_line_tag(item)
                        self._native_img_map[tag] = item[key]
                        self.logger.debug(f"[MinerU] Mapped native image: {tag} → {item[key]}")
                    except Exception as e:
                        self.logger.debug(f"[MinerU] Failed to map native image: {e}")
                    
                    break  # 只需要第一个找到的图片路径

        # MinerU(vlm-http-client) 不会为纯文本生成图片，这里兜底用本地页图裁剪生成，方便后续引用/MinIO 存图
        try:
            self._generate_missing_images(data, subdir, file_stem)
        except Exception as e:
            self.logger.warning(f"[MinerU] generate missing images failed: {e}")

        return data

    def _transfer_to_sections(self, outputs: list[dict[str, Any]], parse_method: str = None):
        sections = []
        for output in outputs:
            match output["type"]:
                case MinerUContentType.TEXT:
                    section = output["text"]
                case MinerUContentType.TABLE:
                    section = output.get("table_body", "") + "\n".join(output.get("table_caption", [])) + "\n".join(output.get("table_footnote", []))
                    if not section.strip():
                        section = "FAILED TO PARSE TABLE"
                case MinerUContentType.IMAGE:
                    section = "".join(output.get("image_caption", [])) + "\n" + "".join(output.get("image_footnote", []))
                case MinerUContentType.EQUATION:
                    section = output["text"]
                case MinerUContentType.CODE:
                    section = output["code_body"] + "\n".join(output.get("code_caption", []))
                case MinerUContentType.LIST:
                    section = "\n".join(output.get("list_items", []))
                case MinerUContentType.DISCARDED:
                    pass

            if section and parse_method == "manual":
                sections.append((section, output["type"], self._raw_line_tag(output)))
            elif section and parse_method == "paper":
                sections.append((section + self._raw_line_tag(output), output["type"]))
            else:
                sections.append((section, self._raw_line_tag(output)))
        return sections

    def _transfer_to_tables(self, outputs: list[dict[str, Any]]):
        return []

    def parse_pdf(
        self,
        filepath: str | PathLike[str],
        binary: BytesIO | bytes,
        callback: Optional[Callable] = None,
        *,
        output_dir: Optional[str] = None,
        backend: str = "pipeline",
        lang: Optional[str] = None,
        method: str = "auto",
        server_url: Optional[str] = None,
        delete_output: bool = True,
        parse_method: str = "raw",
    ) -> tuple:
        import shutil

        temp_pdf = None
        created_tmp_dir = False
        # per-task cache reset to avoid stale images across documents
        self._img_path_cache = {}
        self._native_img_map = {}

        # remove spaces, or mineru crash, and _read_output fail too
        file_path = Path(filepath)
        pdf_file_name = file_path.stem.replace(" ", "") + ".pdf"
        pdf_file_path_valid = os.path.join(file_path.parent, pdf_file_name)

        if binary:
            temp_dir = Path(tempfile.mkdtemp(prefix="mineru_bin_pdf_"))
            temp_pdf = temp_dir / pdf_file_name
            with open(temp_pdf, "wb") as f:
                f.write(binary)
            pdf = temp_pdf
            self.logger.info(f"[MinerU] Received binary PDF -> {temp_pdf}")
            if callback:
                callback(0.15, f"[MinerU] Received binary PDF -> {temp_pdf}")
        else:
            if pdf_file_path_valid != filepath:
                self.logger.info(f"[MinerU] Remove all space in file name: {pdf_file_path_valid}")
                shutil.move(filepath, pdf_file_path_valid)
            pdf = Path(pdf_file_path_valid)
            if not pdf.exists():
                if callback:
                    callback(-1, f"[MinerU] PDF not found: {pdf}")
                raise FileNotFoundError(f"[MinerU] PDF not found: {pdf}")

        if output_dir:
            out_dir = Path(output_dir)
            out_dir.mkdir(parents=True, exist_ok=True)
        else:
            out_dir = Path(tempfile.mkdtemp(prefix="mineru_pdf_"))
            created_tmp_dir = True

        self.logger.info(f"[MinerU] Output directory: {out_dir}")
        if callback:
            callback(0.15, f"[MinerU] Output directory: {out_dir}")

        self.__images__(pdf, zoomin=1)

        try:
            self._run_mineru(pdf, out_dir, method=method, backend=backend, lang=lang, server_url=server_url, callback=callback)
            outputs = self._read_output(out_dir, pdf.stem, method=method, backend=backend)
            self.logger.info(f"[MinerU] Parsed {len(outputs)} blocks from PDF.")
            if callback:
                callback(0.75, f"[MinerU] Parsed {len(outputs)} blocks from PDF.")

            return self._transfer_to_sections(outputs, parse_method), self._transfer_to_tables(outputs)
        finally:
            if temp_pdf and temp_pdf.exists():
                try:
                    temp_pdf.unlink()
                    temp_pdf.parent.rmdir()
                except Exception:
                    pass
            if delete_output and created_tmp_dir and out_dir.exists():
                try:
                    shutil.rmtree(out_dir)
                except Exception:
                    pass


if __name__ == "__main__":
    parser = MinerUParser("mineru")
    ok, reason = parser.check_installation()
    print("MinerU available:", ok)

    filepath = ""
    with open(filepath, "rb") as file:
        outputs = parser.parse_pdf(filepath=filepath, binary=file.read())
        for output in outputs:
            print(output)
