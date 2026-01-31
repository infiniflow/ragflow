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
"""
DeepSeek-OCR2 Parser for RAGFlow

This parser uses DeepSeek-OCR2 (Visual Causal Flow technology) for document parsing.
Model: https://huggingface.co/deepseek-ai/DeepSeek-OCR-2
"""
import base64
import json
import logging
from io import BytesIO
from os import PathLike
from pathlib import Path
from typing import Callable, Optional, List, Tuple, Any

try:
    import fitz  # PyMuPDF
except ImportError:
    fitz = None

try:
    import requests
except ImportError:
    requests = None

from PIL import Image

from deepdoc.parser.pdf_parser import RAGFlowPdfParser


class DeepSeekOcr2Backend:
    """Backend types for DeepSeek-OCR2 parser."""
    LOCAL = "local"
    HTTP = "http"


class DeepSeekOcr2Parser(RAGFlowPdfParser):
    """
    DeepSeek-OCR2 Parser for PDF document parsing.
    
    Uses Visual Causal Flow technology for high-accuracy document understanding.
    Supports document parsing and free OCR modes.
    """
    
    DEFAULT_MODEL = "deepseek-ai/DeepSeek-OCR-2"
    
    # Prompts for different parsing modes
    PROMPT_DOCUMENT_PARSE = "<image>\n<|grounding|>Convert the document to markdown."
    PROMPT_FREE_OCR = "<image>\nFree OCR."
    
    def __init__(
        self,
        model_path: Optional[str] = None,
        device: str = "cuda",
        use_flash_attn: bool = True,
        backend: str = DeepSeekOcr2Backend.LOCAL,
        api_url: Optional[str] = None,
        api_key: Optional[str] = None,
    ):
        """
        Initialize the DeepSeek-OCR2 parser.
        
        Args:
            model_path: Path to local model or HuggingFace model ID.
                       Defaults to DEFAULT_MODEL if not specified.
            device: Device to run inference on ('cuda', 'cpu', 'mps').
            use_flash_attn: Whether to use Flash Attention 2 for faster inference.
            backend: Backend to use - 'local' for local model, 'http' for HTTP API.
            api_url: URL of HTTP API endpoint (required when backend='http').
            api_key: API key for HTTP authentication (optional).
        """
        self.model_path = model_path or self.DEFAULT_MODEL
        self.model_name = self.DEFAULT_MODEL
        self.device = device
        self.use_flash_attn = use_flash_attn
        self.backend = backend
        self.api_url = api_url
        self.api_key = api_key
        
        self.logger = logging.getLogger(self.__class__.__name__)
        
        # Lazy loading - model and processor loaded on first use
        self._model = None
        self._processor = None
        self._tokenizer = None
        
    def _load_model(self):
        """
        Lazy load the model and tokenizer.
        
        Uses HuggingFace Transformers to load the DeepSeek-OCR2 model.
        Note: DeepSeek-OCR2 uses AutoModel and AutoTokenizer with custom inference.
        """
        if self._model is not None:
            return
            
        self.logger.info(f"Loading DeepSeek-OCR2 model from {self.model_path}...")
        
        try:
            import torch
            from transformers import AutoModel, AutoTokenizer
            
            # Determine attention implementation
            attn_impl = "flash_attention_2" if self.use_flash_attn else "eager"
            
            # Load tokenizer (required for model.infer())
            self._tokenizer = AutoTokenizer.from_pretrained(
                self.model_path, 
                trust_remote_code=True
            )
            
            # Load model with appropriate settings (following official API)
            self._model = AutoModel.from_pretrained(
                self.model_path,
                _attn_implementation=attn_impl,
                trust_remote_code=True,
                use_safetensors=True,
            )
            
            # Move to device and set dtype
            if self.device == "cuda":
                self._model = self._model.cuda().to(torch.bfloat16)
            elif self.device == "mps":
                self._model = self._model.to("mps")
            else:
                self._model = self._model.to(torch.float32)
                
            self._model.eval()
            self.logger.info("DeepSeek-OCR2 model loaded successfully.")
            
        except ImportError as e:
            raise ImportError(
                "DeepSeek-OCR2 requires torch and transformers. "
                "Install with: pip install torch transformers"
            ) from e
        except Exception as e:
            self.logger.error(f"Failed to load DeepSeek-OCR2 model: {e}")
            raise
            
    def _pdf_to_images(
        self,
        pdf_path: str | PathLike[str] | None = None,
        pdf_binary: bytes | BytesIO | None = None,
        dpi: int = 144,
        page_from: int = 0,
        page_to: int = 9999,
    ) -> List[Image.Image]:
        """
        Convert PDF pages to PIL Images using PyMuPDF (fitz).
        
        Args:
            pdf_path: Path to PDF file.
            pdf_binary: Binary PDF data.
            dpi: Resolution for rendering (default 144).
            page_from: First page to process (0-indexed).
            page_to: Last page to process (inclusive).
            
        Returns:
            List of PIL Images, one per page.
        """
        if fitz is None:
            raise ImportError("PyMuPDF (fitz) is required for PDF processing. Install with: pip install pymupdf")
            
        images = []
        
        # Open PDF
        if pdf_binary is not None:
            if isinstance(pdf_binary, BytesIO):
                pdf_binary = pdf_binary.read()
            doc = fitz.open(stream=pdf_binary, filetype="pdf")
        elif pdf_path is not None:
            doc = fitz.open(str(pdf_path))
        else:
            raise ValueError("Either pdf_path or pdf_binary must be provided")
            
        try:
            # Calculate zoom factor for desired DPI (fitz default is 72 DPI)
            zoom = dpi / 72.0
            matrix = fitz.Matrix(zoom, zoom)
            
            # Process pages
            end_page = min(page_to + 1, len(doc))
            for page_num in range(page_from, end_page):
                page = doc[page_num]
                pix = page.get_pixmap(matrix=matrix)
                
                # Convert to PIL Image
                img = Image.frombytes("RGB", [pix.width, pix.height], pix.samples)
                images.append(img)
                
        finally:
            doc.close()
            
        return images
    
    def check_available(self) -> Tuple[bool, str]:
        """
        Check if the parser backend is available and properly configured.
        
        Returns:
            Tuple of (is_available, reason).
        """
        if self.backend == DeepSeekOcr2Backend.HTTP:
            if not self.api_url:
                return False, "HTTP backend: API URL not configured"
            if requests is None:
                return False, "HTTP backend: requests library not installed"
            try:
                # Try to reach the API endpoint with a short timeout
                response = requests.get(
                    self.api_url.rstrip('/').rsplit('/v1/ocr', 1)[0] + '/health',
                    timeout=5
                )
                if 200 <= response.status_code < 300:
                    return True, ""
                return False, f"HTTP backend: Unexpected status code ({response.status_code})"
            except requests.exceptions.ConnectionError:
                return False, "HTTP backend: API URL not accessible"
            except requests.exceptions.Timeout:
                return False, "HTTP backend: API URL not accessible (timeout)"
            except Exception as e:
                return False, f"HTTP backend: API URL not accessible ({e})"
        else:
            # Local backend - check dependencies
            return self.check_installation()
    
    def _ocr_image_http(
        self,
        image: Image.Image,
        prompt: str = None,
        max_new_tokens: int = 8192,
    ) -> str:
        """
        Perform OCR on a single image using HTTP API.
        
        Args:
            image: PIL Image to process.
            prompt: Prompt to use. Defaults to PROMPT_DOCUMENT_PARSE.
            max_new_tokens: Maximum tokens to generate.
            
        Returns:
            Extracted text/markdown from the image.
        """
        if requests is None:
            raise ImportError("requests library required for HTTP backend. Install with: pip install requests")
            
        if not self.api_url:
            raise ValueError("api_url must be specified for HTTP backend")
            
        if prompt is None:
            prompt = self.PROMPT_DOCUMENT_PARSE
            
        # Encode image as base64
        buffer = BytesIO()
        image.save(buffer, format="PNG")
        image_base64 = base64.b64encode(buffer.getvalue()).decode("utf-8")
        
        # Prepare request payload
        payload = {
            "image": image_base64,
            "prompt": prompt,
            "max_new_tokens": max_new_tokens,
        }
        
        # Prepare headers
        headers = {"Content-Type": "application/json"}
        if self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"
            
        try:
            response = requests.post(
                self.api_url,
                json=payload,
                headers=headers,
                timeout=120,
            )
            response.raise_for_status()
            
            try:
                result = response.json()
            except json.JSONDecodeError as e:
                raise RuntimeError(f"HTTP backend returned invalid JSON: {response.text[:200]}") from e
            return result.get("text", result.get("result", ""))
            
        except requests.exceptions.RequestException as e:
            self.logger.error(f"HTTP OCR request failed: {e}")
            raise
        
    def _ocr_image(
        self,
        image: Image.Image,
        prompt: str = None,
        max_new_tokens: int = 8192,
    ) -> str:
        """
        Perform OCR on a single image.
        
        Args:
            image: PIL Image to process.
            prompt: Prompt to use. Defaults to PROMPT_DOCUMENT_PARSE.
            max_new_tokens: Maximum tokens to generate.
            
        Returns:
            Extracted text/markdown from the image.
        """
        # Dispatch based on backend
        if self.backend == DeepSeekOcr2Backend.HTTP:
            return self._ocr_image_http(image, prompt, max_new_tokens)
        
        # Local backend
        self._load_model()
        
        if prompt is None:
            prompt = self.PROMPT_DOCUMENT_PARSE
            
        try:
            import tempfile
            import os
            
            # DeepSeek-OCR2 uses model.infer() which requires image file path
            # Save image to temporary file
            with tempfile.NamedTemporaryFile(suffix='.png', delete=False) as tmp_file:
                image.save(tmp_file, format='PNG')
                tmp_path = tmp_file.name
            
            try:
                # Use official model.infer() API
                # Parameters from official documentation:
                # - base_size: 1024 (global view size)
                # - image_size: 768 (crop size)
                # - crop_mode: True (enable dynamic resolution)
                result = self._model.infer(
                    self._tokenizer,
                    prompt=prompt,
                    image_file=tmp_path,
                    base_size=1024,
                    image_size=768,
                    crop_mode=True,
                    save_results=False,  # Don't save to disk
                )
                
                # Result is the OCR output text
                if isinstance(result, str):
                    return result
                elif isinstance(result, dict):
                    return result.get('text', result.get('result', str(result)))
                else:
                    return str(result)
                    
            finally:
                # Clean up temporary file
                if os.path.exists(tmp_path):
                    os.unlink(tmp_path)
            
        except Exception as e:
            self.logger.error(f"OCR failed for image: {e}")
            raise
            
    def parse_pdf(
        self,
        filepath: str | PathLike[str] | None = None,
        binary: bytes | BytesIO | None = None,
        callback: Optional[Callable[[float, str], None]] = None,
        *,
        page_from: int = 0,
        page_to: int = 9999,
        dpi: int = 144,
        mode: str = "document",
        **kwargs,
    ) -> Tuple[List[Tuple[str, str]], List[Any]]:
        """
        Parse a PDF document using DeepSeek-OCR2.
        
        Args:
            filepath: Path to PDF file.
            binary: Binary PDF data.
            callback: Progress callback function(progress: float, message: str).
            page_from: First page to process (0-indexed).
            page_to: Last page to process (inclusive).
            dpi: Resolution for rendering pages.
            mode: Parsing mode - 'document' for structured markdown, 'ocr' for free OCR.
            **kwargs: Additional arguments (ignored for compatibility).
            
        Returns:
            Tuple of (sections, tables) where:
            - sections: List of (text, position_tag) tuples
            - tables: List of extracted tables (currently empty)
        """
        self.logger.info(f"Parsing PDF with DeepSeek-OCR2: {filepath or 'binary'}")
        
        if callback:
            callback(0.05, "Converting PDF to images...")
            
        # Convert PDF to images
        images = self._pdf_to_images(
            pdf_path=filepath,
            pdf_binary=binary,
            dpi=dpi,
            page_from=page_from,
            page_to=page_to,
        )
        
        if callback:
            callback(0.15, f"Processing {len(images)} pages with DeepSeek-OCR2...")
            
        # Select prompt based on mode
        prompt = self.PROMPT_FREE_OCR if mode == "ocr" else self.PROMPT_DOCUMENT_PARSE
        
        sections = []
        total_pages = len(images)
        
        for i, img in enumerate(images):
            if callback:
                progress = 0.15 + (0.80 * (i / total_pages))
                callback(progress, f"Processing page {i + 1}/{total_pages}...")
                
            try:
                text = self._ocr_image(img, prompt=prompt)
                
                # Create position tag (page number, coordinates placeholder)
                page_num = page_from + i + 1
                position_tag = f"@@{page_num}\t0.0\t0.0\t0.0\t0.0##"
                
                if text.strip():
                    sections.append((text, position_tag))
                    
            except Exception as e:
                self.logger.warning(f"Failed to process page {i + 1}: {e}")
                continue
                
        if callback:
            callback(0.95, "Parsing complete.")
            
        # Currently not extracting tables separately
        tables = []
        
        self.logger.info(f"Extracted {len(sections)} sections from PDF.")
        
        return sections, tables
        
    def check_installation(self) -> Tuple[bool, str]:
        """
        Check if DeepSeek-OCR2 dependencies are installed.
        
        Returns:
            Tuple of (is_available, reason).
        """
        errors = []
        
        if fitz is None:
            errors.append("PyMuPDF not installed (pip install pymupdf)")
            
        try:
            import torch
        except ImportError:
            errors.append("PyTorch not installed (pip install torch)")
            
        try:
            import transformers
        except ImportError:
            errors.append("Transformers not installed (pip install transformers)")
            
        if errors:
            return False, "; ".join(errors)
            
        return True, ""
