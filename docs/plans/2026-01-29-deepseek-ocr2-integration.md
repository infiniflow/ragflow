# DeepSeek-OCR2 Integration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 为 RAGflow 添加 DeepSeek-OCR2 作为新的 OCR 引擎选项，支持基于 Visual Causal Flow 的智能文档解析。

**Architecture:** 
- 遵循 RAGflow 现有的 OCR 工厂模式，在 `rag/llm/ocr_model.py` 中创建 `DeepSeekOcr2Model` 类
- 支持两种后端：HuggingFace Transformers（本地部署）和 HTTP API（远程服务）
- 复用 `deepdoc/parser` 的输出格式，确保与现有解析管道兼容

**Tech Stack:** 
- Python 3.12+, PyTorch, Transformers 4.46+
- HuggingFace Model: `deepseek-ai/DeepSeek-OCR-2`
- CUDA 11.8+ (GPU 推理), Flash Attention 2

---

## Task 1: 创建 DeepSeek-OCR2 解析器基础类

**Files:**
- Create: `deepdoc/parser/deepseek_ocr2_parser.py`
- Test: `test/deepdoc/parser/test_deepseek_ocr2_parser.py`

**Step 1: Write the failing test**

```python
# test/deepdoc/parser/test_deepseek_ocr2_parser.py
import pytest
from deepdoc.parser.deepseek_ocr2_parser import DeepSeekOcr2Parser

def test_parser_init():
    """Test parser initialization with default config."""
    parser = DeepSeekOcr2Parser()
    assert parser is not None
    assert parser.model_name == "deepseek-ai/DeepSeek-OCR-2"

def test_parser_init_custom_model():
    """Test parser initialization with custom model path."""
    parser = DeepSeekOcr2Parser(model_path="/custom/path")
    assert parser.model_path == "/custom/path"
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/weixiaofeng/Desktop/zxwl/coding/ragflow/.worktrees/feature-deepseek-ocr2 && python -m pytest test/deepdoc/parser/test_deepseek_ocr2_parser.py -v`
Expected: FAIL with "ModuleNotFoundError: No module named 'deepdoc.parser.deepseek_ocr2_parser'"

**Step 3: Write minimal implementation**

```python
# deepdoc/parser/deepseek_ocr2_parser.py
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
import os
import tempfile
from io import BytesIO
from pathlib import Path
from typing import Any, Callable, Optional

import fitz  # PyMuPDF
from PIL import Image

from deepdoc.parser.pdf_parser import RAGFlowPdfParser


class DeepSeekOcr2Parser(RAGFlowPdfParser):
    """Parser using DeepSeek-OCR2 model for document understanding."""

    DEFAULT_MODEL = "deepseek-ai/DeepSeek-OCR-2"
    
    def __init__(
        self,
        model_path: Optional[str] = None,
        device: str = "cuda",
        use_flash_attn: bool = True,
    ):
        self.model_name = self.DEFAULT_MODEL
        self.model_path = model_path or self.model_name
        self.device = device
        self.use_flash_attn = use_flash_attn
        self.model = None
        self.tokenizer = None
        self.logger = logging.getLogger(self.__class__.__name__)
        self.outlines = []

    def _load_model(self):
        """Lazy load model on first use."""
        if self.model is not None:
            return
        
        import torch
        from transformers import AutoModel, AutoTokenizer

        self.logger.info(f"[DeepSeek-OCR2] Loading model from {self.model_path}...")
        
        self.tokenizer = AutoTokenizer.from_pretrained(
            self.model_path, trust_remote_code=True
        )
        
        attn_impl = "flash_attention_2" if self.use_flash_attn else "eager"
        self.model = AutoModel.from_pretrained(
            self.model_path,
            _attn_implementation=attn_impl,
            trust_remote_code=True,
            use_safetensors=True,
        )
        self.model = self.model.eval().to(self.device).to(torch.bfloat16)
        self.logger.info("[DeepSeek-OCR2] Model loaded successfully.")

    def _pdf_to_images(self, pdf_path: str, dpi: int = 150) -> list[Image.Image]:
        """Convert PDF pages to PIL Images."""
        images = []
        doc = fitz.open(pdf_path)
        for page_num in range(len(doc)):
            page = doc[page_num]
            mat = fitz.Matrix(dpi / 72, dpi / 72)
            pix = page.get_pixmap(matrix=mat)
            img = Image.frombytes("RGB", [pix.width, pix.height], pix.samples)
            images.append(img)
        doc.close()
        return images

    def _ocr_image(self, image_path: str, prompt: str = None) -> str:
        """Run OCR on a single image."""
        self._load_model()
        
        if prompt is None:
            prompt = "<image>\n<|grounding|>Convert the document to markdown. "
        
        with tempfile.TemporaryDirectory() as tmpdir:
            result = self.model.infer(
                self.tokenizer,
                prompt=prompt,
                image_file=image_path,
                output_path=tmpdir,
                base_size=1024,
                image_size=768,
                crop_mode=True,
                save_results=False,
            )
        return result

    def parse_pdf(
        self,
        filepath: str,
        binary: bytes = None,
        callback: Optional[Callable] = None,
        **kwargs,
    ) -> tuple[list, list]:
        """Parse PDF using DeepSeek-OCR2."""
        self._load_model()
        
        # Handle binary input
        if binary:
            with tempfile.NamedTemporaryFile(suffix=".pdf", delete=False) as f:
                f.write(binary)
                filepath = f.name
        
        try:
            images = self._pdf_to_images(filepath)
            sections = []
            tables = []
            
            total_pages = len(images)
            for i, img in enumerate(images):
                if callback:
                    progress = 0.1 + (0.8 * i / total_pages)
                    callback(progress, f"[DeepSeek-OCR2] Processing page {i+1}/{total_pages}")
                
                # Save image temporarily
                with tempfile.NamedTemporaryFile(suffix=".png", delete=False) as f:
                    img.save(f.name)
                    result = self._ocr_image(f.name)
                    os.unlink(f.name)
                
                # Format as section with position tag
                position_tag = f"@@{i+1}\t0\t{img.width}\t0\t{img.height}##"
                sections.append((result, position_tag))
            
            if callback:
                callback(0.95, "[DeepSeek-OCR2] Parsing complete")
            
            return sections, tables
            
        finally:
            if binary and os.path.exists(filepath):
                os.unlink(filepath)
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/weixiaofeng/Desktop/zxwl/coding/ragflow/.worktrees/feature-deepseek-ocr2 && python -m pytest test/deepdoc/parser/test_deepseek_ocr2_parser.py -v`
Expected: PASS

**Step 5: Commit**

```bash
git add deepdoc/parser/deepseek_ocr2_parser.py test/deepdoc/parser/test_deepseek_ocr2_parser.py
git commit -m "feat(deepdoc): add DeepSeek-OCR2 parser base class"
```

---

## Task 2: 创建 HTTP API 后端支持

**Files:**
- Modify: `deepdoc/parser/deepseek_ocr2_parser.py`
- Test: `test/deepdoc/parser/test_deepseek_ocr2_parser.py`

**Step 1: Write the failing test**

```python
# 添加到 test/deepdoc/parser/test_deepseek_ocr2_parser.py

def test_http_backend_init():
    """Test HTTP backend initialization."""
    parser = DeepSeekOcr2Parser(
        backend="http",
        api_url="http://localhost:8000/v1/ocr",
        api_key="test-key"
    )
    assert parser.backend == "http"
    assert parser.api_url == "http://localhost:8000/v1/ocr"

def test_check_http_availability():
    """Test HTTP endpoint availability check."""
    parser = DeepSeekOcr2Parser(
        backend="http",
        api_url="http://invalid-url:9999/v1/ocr"
    )
    available, reason = parser.check_available()
    assert available is False
    assert "not accessible" in reason.lower()
```

**Step 2: Run test to verify it fails**

Run: `python -m pytest test/deepdoc/parser/test_deepseek_ocr2_parser.py::test_http_backend_init -v`
Expected: FAIL with "TypeError: __init__() got an unexpected keyword argument 'backend'"

**Step 3: Write implementation**

```python
# 更新 deepdoc/parser/deepseek_ocr2_parser.py 的 __init__ 和添加新方法

class DeepSeekOcr2Backend:
    """Backend options for DeepSeek-OCR2."""
    LOCAL = "local"  # Local HuggingFace Transformers
    HTTP = "http"    # Remote HTTP API


class DeepSeekOcr2Parser(RAGFlowPdfParser):
    # ... 保留现有代码 ...

    def __init__(
        self,
        model_path: Optional[str] = None,
        device: str = "cuda",
        use_flash_attn: bool = True,
        backend: str = DeepSeekOcr2Backend.LOCAL,
        api_url: Optional[str] = None,
        api_key: Optional[str] = None,
    ):
        self.model_name = self.DEFAULT_MODEL
        self.model_path = model_path or self.model_name
        self.device = device
        self.use_flash_attn = use_flash_attn
        self.backend = backend
        self.api_url = api_url
        self.api_key = api_key
        self.model = None
        self.tokenizer = None
        self.logger = logging.getLogger(self.__class__.__name__)
        self.outlines = []

    def check_available(self) -> tuple[bool, str]:
        """Check if the backend is available."""
        if self.backend == DeepSeekOcr2Backend.HTTP:
            return self._check_http_available()
        else:
            return self._check_local_available()

    def _check_http_available(self) -> tuple[bool, str]:
        """Check HTTP API availability."""
        if not self.api_url:
            return False, "[DeepSeek-OCR2] HTTP API URL not configured"
        
        import requests
        try:
            response = requests.head(self.api_url, timeout=5)
            if response.status_code in [200, 301, 302, 307, 308, 405]:
                return True, ""
            return False, f"[DeepSeek-OCR2] HTTP API not accessible: status {response.status_code}"
        except Exception as e:
            return False, f"[DeepSeek-OCR2] HTTP API not accessible: {e}"

    def _check_local_available(self) -> tuple[bool, str]:
        """Check local model availability."""
        try:
            import torch
            if not torch.cuda.is_available() and self.device == "cuda":
                return False, "[DeepSeek-OCR2] CUDA not available"
            return True, ""
        except ImportError:
            return False, "[DeepSeek-OCR2] PyTorch not installed"

    def _ocr_image_http(self, image_path: str, prompt: str = None) -> str:
        """Run OCR via HTTP API."""
        import base64
        import requests
        
        if prompt is None:
            prompt = "<image>\n<|grounding|>Convert the document to markdown. "
        
        with open(image_path, "rb") as f:
            image_data = base64.b64encode(f.read()).decode("utf-8")
        
        headers = {"Content-Type": "application/json"}
        if self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"
        
        payload = {
            "image": image_data,
            "prompt": prompt,
            "base_size": 1024,
            "image_size": 768,
            "crop_mode": True,
        }
        
        response = requests.post(self.api_url, json=payload, headers=headers, timeout=300)
        response.raise_for_status()
        return response.json().get("text", "")
```

**Step 4: Run test to verify it passes**

Run: `python -m pytest test/deepdoc/parser/test_deepseek_ocr2_parser.py -v`
Expected: PASS

**Step 5: Commit**

```bash
git add deepdoc/parser/deepseek_ocr2_parser.py test/deepdoc/parser/test_deepseek_ocr2_parser.py
git commit -m "feat(deepdoc): add HTTP backend support for DeepSeek-OCR2"
```

---

## Task 3: 注册 OCR 模型工厂

**Files:**
- Modify: `rag/llm/ocr_model.py`
- Modify: `conf/llm_factories.json`
- Test: `test/rag/llm/test_ocr_model.py`

**Step 1: Write the failing test**

```python
# test/rag/llm/test_ocr_model.py
import pytest
from rag.llm import OcrModel

def test_deepseek_ocr2_registered():
    """Test DeepSeek-OCR2 is registered in OcrModel."""
    assert "DeepSeek-OCR2" in OcrModel
    
def test_deepseek_ocr2_instantiation():
    """Test DeepSeek-OCR2 model can be instantiated."""
    model_class = OcrModel.get("DeepSeek-OCR2")
    assert model_class is not None
    assert hasattr(model_class, "_FACTORY_NAME")
    assert model_class._FACTORY_NAME == "DeepSeek-OCR2"
```

**Step 2: Run test to verify it fails**

Run: `python -m pytest test/rag/llm/test_ocr_model.py::test_deepseek_ocr2_registered -v`
Expected: FAIL with KeyError or assertion error

**Step 3: Write implementation**

```python
# 添加到 rag/llm/ocr_model.py

from deepdoc.parser.deepseek_ocr2_parser import DeepSeekOcr2Parser, DeepSeekOcr2Backend


class DeepSeekOcr2Model(Base):
    """DeepSeek-OCR2 model for document understanding with Visual Causal Flow."""
    
    _FACTORY_NAME = "DeepSeek-OCR2"

    def __init__(self, key: str | dict, model_name: str, **kwargs):
        Base.__init__(self, key, model_name, **kwargs)
        
        # Parse configuration
        raw_config = {}
        if key:
            try:
                raw_config = json.loads(key) if isinstance(key, str) else key
            except Exception:
                raw_config = {}
        
        config = raw_config.get("api_key", raw_config)
        if not isinstance(config, dict):
            config = {}
        
        def _resolve_config(key: str, env_key: str, default=""):
            return config.get(key, config.get(env_key, os.environ.get(env_key, default)))
        
        # Configuration options
        self.backend = _resolve_config("backend", "DEEPSEEK_OCR2_BACKEND", "local")
        self.api_url = _resolve_config("api_url", "DEEPSEEK_OCR2_API_URL", "")
        self.api_key_value = _resolve_config("api_key", "DEEPSEEK_OCR2_API_KEY", "")
        self.model_path = _resolve_config("model_path", "DEEPSEEK_OCR2_MODEL_PATH", "")
        self.device = _resolve_config("device", "DEEPSEEK_OCR2_DEVICE", "cuda")
        self.use_flash_attn = bool(int(_resolve_config("use_flash_attn", "DEEPSEEK_OCR2_USE_FLASH_ATTN", "1")))
        
        # Initialize parser
        self.parser = DeepSeekOcr2Parser(
            model_path=self.model_path or None,
            device=self.device,
            use_flash_attn=self.use_flash_attn,
            backend=self.backend,
            api_url=self.api_url,
            api_key=self.api_key_value,
        )
        
        # Log config (redacted)
        redacted = {k: "[REDACTED]" if "key" in k.lower() else v for k, v in config.items()}
        logging.info(f"[DeepSeek-OCR2] Config: {redacted}")

    def check_available(self) -> tuple[bool, str]:
        """Check if DeepSeek-OCR2 is available."""
        return self.parser.check_available()

    def parse_pdf(self, filepath: str, binary=None, callback=None, **kwargs):
        """Parse PDF using DeepSeek-OCR2."""
        ok, reason = self.check_available()
        if not ok:
            raise RuntimeError(f"DeepSeek-OCR2 not available: {reason}")
        
        return self.parser.parse_pdf(
            filepath=filepath,
            binary=binary,
            callback=callback,
            **kwargs
        )
```

**Step 4: Update llm_factories.json**

```json
// 在 conf/llm_factories.json 的 factory_llm_infos 数组末尾添加（MinerU 之前）:
{
    "name": "DeepSeek-OCR2",
    "logo": "",
    "tags": "OCR",
    "status": "1",
    "rank": "898",
    "llm": []
}
```

**Step 5: Run test to verify it passes**

Run: `python -m pytest test/rag/llm/test_ocr_model.py -v`
Expected: PASS

**Step 6: Commit**

```bash
git add rag/llm/ocr_model.py conf/llm_factories.json test/rag/llm/test_ocr_model.py
git commit -m "feat(rag): register DeepSeek-OCR2 as OCR model factory"
```

---

## Task 4: 添加依赖配置

**Files:**
- Modify: `pyproject.toml`
- Create: `requirements-deepseek-ocr2.txt`

**Step 1: Create optional dependencies file**

```txt
# requirements-deepseek-ocr2.txt
# Optional dependencies for DeepSeek-OCR2 support
transformers>=4.46.3
tokenizers>=0.20.3
torch>=2.6.0
PyMuPDF>=1.24.0
einops
Pillow
numpy
# Optional: flash-attn>=2.7.3 (requires manual installation)
```

**Step 2: Update pyproject.toml**

在 `[project.optional-dependencies]` 部分添加：

```toml
deepseek-ocr2 = [
    "transformers>=4.46.3",
    "tokenizers>=0.20.3", 
    "PyMuPDF>=1.24.0",
    "einops",
]
```

**Step 3: Commit**

```bash
git add pyproject.toml requirements-deepseek-ocr2.txt
git commit -m "chore: add DeepSeek-OCR2 optional dependencies"
```

---

## Task 5: 添加文档

**Files:**
- Modify: `docs/references/supported_models.mdx`
- Create: `docs/guides/dataset/deepseek_ocr2_setup.md`

**Step 1: Update supported models doc**

在 OCR 模型部分添加：

```markdown
### DeepSeek-OCR2

DeepSeek-OCR2 使用 Visual Causal Flow 技术，模拟人类的跳跃式阅读逻辑，特别适合复杂版面（报纸、多栏论文、图表）的文档理解。

| Feature | Value |
|---------|-------|
| Provider | DeepSeek AI |
| Model | deepseek-ai/DeepSeek-OCR-2 |
| Backend | Local (Transformers) / HTTP API |
| VRAM | ~16GB (推荐) |

**配置选项：**
- `backend`: `local` (本地推理) 或 `http` (远程 API)
- `api_url`: HTTP API 地址 (仅 http 后端)
- `api_key`: API 密钥 (仅 http 后端)
- `model_path`: 自定义模型路径 (可选)
- `device`: `cuda` 或 `cpu`
- `use_flash_attn`: 是否使用 Flash Attention (默认 true)
```

**Step 2: Create setup guide**

```markdown
# DeepSeek-OCR2 Setup Guide

## Prerequisites

- CUDA 11.8+
- GPU with 16GB+ VRAM (recommended)
- Python 3.12+

## Installation

### Option 1: Local Deployment

1. Install dependencies:
   ```bash
   pip install -r requirements-deepseek-ocr2.txt
   pip install flash-attn==2.7.3 --no-build-isolation
   ```

2. The model will auto-download from HuggingFace on first use.

### Option 2: HTTP API Backend

Configure environment variables:
```bash
export DEEPSEEK_OCR2_BACKEND=http
export DEEPSEEK_OCR2_API_URL=http://your-server:8000/v1/ocr
export DEEPSEEK_OCR2_API_KEY=your-api-key
```

## Usage in RAGflow

1. Go to **Settings > Model Providers**
2. Add **DeepSeek-OCR2** provider
3. Configure backend and credentials
4. Select DeepSeek-OCR2 as PDF parser in dataset settings
```

**Step 3: Commit**

```bash
git add docs/
git commit -m "docs: add DeepSeek-OCR2 documentation"
```

---

## Task 6: 集成测试

**Files:**
- Create: `test/integration/test_deepseek_ocr2_integration.py`

**Step 1: Write integration test**

```python
# test/integration/test_deepseek_ocr2_integration.py
import os
import pytest
from unittest.mock import MagicMock, patch

from rag.llm import OcrModel


@pytest.fixture
def mock_model():
    """Mock DeepSeek model for testing without GPU."""
    with patch("deepdoc.parser.deepseek_ocr2_parser.AutoModel") as mock_auto:
        with patch("deepdoc.parser.deepseek_ocr2_parser.AutoTokenizer") as mock_tok:
            mock_tok.from_pretrained.return_value = MagicMock()
            mock_model = MagicMock()
            mock_model.infer.return_value = "# Test Document\n\nThis is test content."
            mock_auto.from_pretrained.return_value = mock_model
            yield mock_model


def test_end_to_end_parse(mock_model, tmp_path):
    """Test end-to-end PDF parsing."""
    # Create test PDF
    test_pdf = tmp_path / "test.pdf"
    # ... create minimal PDF ...
    
    model_class = OcrModel.get("DeepSeek-OCR2")
    model = model_class(key='{"backend": "local"}', model_name="test")
    
    with patch.object(model.parser, "_load_model"):
        with patch.object(model.parser, "_ocr_image", return_value="Test content"):
            sections, tables = model.parse_pdf(str(test_pdf))
    
    assert isinstance(sections, list)
    assert isinstance(tables, list)
```

**Step 2: Run integration tests**

Run: `python -m pytest test/integration/test_deepseek_ocr2_integration.py -v`
Expected: PASS

**Step 3: Commit**

```bash
git add test/integration/test_deepseek_ocr2_integration.py
git commit -m "test: add DeepSeek-OCR2 integration tests"
```

---

## Summary Checklist

- [ ] Task 1: 创建 DeepSeek-OCR2 解析器基础类
- [ ] Task 2: 创建 HTTP API 后端支持
- [ ] Task 3: 注册 OCR 模型工厂
- [ ] Task 4: 添加依赖配置
- [ ] Task 5: 添加文档
- [ ] Task 6: 集成测试

## Notes

1. **Flash Attention**: 需要手动安装 `flash-attn`，因为它依赖 CUDA 编译
2. **模型大小**: DeepSeek-OCR2 约 7B 参数，需要约 16GB GPU 显存
3. **Visual Causal Flow**: 核心创新是使用 LLM (Qwen2-0.5B) 作为视觉编码器，实现逻辑推理式阅读
4. **Prompts**:
   - 文档解析: `<image>\n<|grounding|>Convert the document to markdown.`
   - 纯文字 OCR: `<image>\nFree OCR.`
