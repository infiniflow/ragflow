#!/usr/bin/env python3
"""
MonkeyOCR Parser for RAGFlow Integration
Integrates CEDD OCR service with RAGFlow document processing
Follows exact flow from cedd_parse.py
"""

import logging
import os
import tempfile
import time
from pathlib import Path
from typing import Dict, Any, Optional, List

# Add monkeyocr to path for CEDD OCR service
import sys
import gc

try:
    import torch
except ImportError:
    torch = None

project_root = Path(__file__).parent.parent.parent
monkeyocr_path = project_root / "monkeyocr"
sys.path.insert(0, str(monkeyocr_path))

# Import the actual cedd_parse function
from monkeyocr.cedd_parse import cedd_parse
from monkeyocr.magic_pdf.model.custom_model import MonkeyOCR

logger = logging.getLogger(__name__)


class MonkeyOCRParser:
    """MonkeyOCR parser for RAGFlow document processing"""

    def __init__(self, config_path: Optional[str] = None):
        """Initialize MonkeyOCR parser"""
        if config_path is None:
            config_path = os.path.join(monkeyocr_path, "model_configs.yaml")

        self.config_path = config_path

    def parse_document(self, file_path: str, output_dir: Optional[str] = None, **kwargs) -> Dict[str, Any]:
        """
        Parse document using MonkeyOCR with single-use approach: Load model → Process → Shutdown completely
        
        This method:
        1. Loads the MonkeyOCR model once
        2. Processes the document with cedd_parse
        3. Completely shuts down and frees all GPU memory
        4. Returns the results

        Args:
            file_path: Input file path
            output_dir: Output directory (optional)
            **kwargs: Additional arguments

        Returns:
            Dict with parsing results
        """
        logger.info(f"📄 parse_document started for file: {file_path}")
        logger.info(f"📁 Output directory: {output_dir}")
        logger.info(f"⚙️ Additional kwargs: {kwargs}")
        
        # Import memory tracking function
        from monkeyocr.magic_pdf.model.custom_model import get_memory_usage
        from monkeyocr.magic_pdf.model.sub_modules.model_init import AtomModelSingleton
        
        # Log initial memory state
        initial_memory = get_memory_usage()
        logger.info(f"Initial memory state: {initial_memory}")
        
        model = None
        try:
            if output_dir is None:
                # Create temporary output directory
                logger.info("📁 Creating temporary output directory...")
                output_dir = tempfile.mkdtemp(prefix="monkeyocr_")
                logger.info(f"✅ Temporary output directory created: {output_dir}")

            logger.info("Starting MonkeyOCR parsing with single-use approach")
            logger.info(f"Input file: {file_path}")
            logger.info(f"Output directory: {output_dir}")

            # Step 1: Load MonkeyOCR model
            logger.info("🚀 Step 1: Loading MonkeyOCR model...")
            start_time = time.time()
            
            model = MonkeyOCR(self.config_path)
            
            load_time = time.time() - start_time
            logger.info(f"Model loaded successfully in {load_time:.2f} seconds")
            
            # Log memory after model loading
            post_load_memory = get_memory_usage()
            logger.info(f"Memory after model load: {post_load_memory}")

            # Step 2: Process document with cedd_parse
            logger.info("🚀 Step 2: Processing document with cedd_parse...")
            start_time = time.time()
            
            enhanced_md_path = cedd_parse(
                input_pdf=file_path, 
                output_dir=output_dir, 
                config_path=self.config_path, 
                MonkeyOCR_model=model,  # Pass the loaded model
                mode="full"
            )
            
            process_time = time.time() - start_time
            logger.info(f"Document processed successfully in {process_time:.2f} seconds")
            logger.info(f"✅ cedd_parse completed, enhanced_md_path: {enhanced_md_path}")

            # Log memory after processing
            post_process_memory = get_memory_usage()
            logger.info(f"Memory after processing: {post_process_memory}")

            # Read the enhanced markdown content
            logger.info("📖 Reading enhanced markdown content...")
            content = self._read_enhanced_markdown(enhanced_md_path)
            logger.info(f"✅ Enhanced markdown content read, length: {len(content)} characters")

            logger.info("MonkeyOCR processing completed successfully")

            result = {"success": True, "parsed_dir": output_dir, "enhanced_md_path": enhanced_md_path, "content": content, "content_list": [content] if content else [], "file_path": file_path}
            logger.info(f"📊 Returning result: success={result['success']}, content_length={len(content)}")
            
            return result

        except Exception as e:
            logger.error(f"❌ Failed to parse document {file_path}: {e}")
            logger.exception(f"Exception details for parse_document:")
            
            return {"success": False, "error": str(e), "file_path": file_path}
        
        finally:
            # Step 3: Complete cleanup and shutdown
            logger.info("🧹 Step 3: Starting complete cleanup and shutdown...")
            start_time = time.time()
            
            try:
                # Clean up the model instance
                if model is not None:
                    logger.info("Cleaning up MonkeyOCR model...")
                    model.cleanup()
                    del model
                    model = None
                
                # Clean up singleton cached models
                logger.info("Cleaning up singleton cached models...")
                singleton = AtomModelSingleton()
                cached_count = singleton.get_cached_model_count()
                if cached_count > 0:
                    logger.info(f"Found {cached_count} cached models, cleaning up...")
                    singleton.cleanup_models()
                else:
                    logger.info("No cached models found in singleton")
                
                # Force garbage collection
                logger.info("Forcing garbage collection...")
                gc.collect()
                
                # Clear GPU cache
                if torch and torch.cuda.is_available():
                    logger.info("Clearing GPU cache...")
                    torch.cuda.empty_cache()
                    torch.cuda.synchronize()
                
                cleanup_time = time.time() - start_time
                logger.info(f"Cleanup completed in {cleanup_time:.2f} seconds")
                
                # Log final memory state
                final_memory = get_memory_usage()
                logger.info(f"Final memory state: {final_memory}")
                
            except Exception as e:
                logger.error(f"Error during cleanup: {e}")
            
            logger.info(f"🏁 parse_document finished for file: {file_path}")

    def _read_enhanced_markdown(self, md_path: str) -> str:
        """Read enhanced markdown content from cedd_parse output"""
        try:
            if os.path.exists(md_path):
                with open(md_path, "r", encoding="utf-8") as f:
                    return f.read()
            else:
                logger.warning(f"Enhanced markdown file not found: {md_path}")
                return ""
        except Exception as e:
            logger.error(f"Failed to read enhanced markdown: {e}")
            return ""

    def get_supported_formats(self) -> List[str]:
        """Get supported file formats"""
        return [".pdf", ".jpg", ".jpeg", ".png", ".tiff", ".bmp"]

    def validate_file(self, file_path: str) -> bool:
        """Validate if file can be processed by MonkeyOCR"""
        try:
            if not os.path.exists(file_path):
                return False

            file_ext = Path(file_path).suffix.lower()
            supported_formats = self.get_supported_formats()
            return file_ext in supported_formats

        except Exception as e:
            logger.error(f"Failed to validate file: {e}")
            return False

    def get_parsing_options(self) -> Dict[str, Any]:
        """Get available parsing options"""
        return {
            "mode": "full",  # full, parse_only, ocr_only
            "split_pages": False,
            "pred_abandon": False,
            "extract_images": True,
            "generate_layout_pdf": True,
            "generate_spans_pdf": True,
        }


def chunk(filename, binary=None, from_page=0, to_page=100000, lang="Chinese", callback=None, **kwargs):
    """
    MonkeyOCR chunk function for RAGFlow integration.
    Follows exact cedd_parse.py flow with 'full' mode.

    Args:
        filename (str): File name
        binary (bytes): File content
        from_page (int): Start page
        to_page (int): End page
        lang (str): Language
        callback (function): Progress callback
        **kwargs: Additional arguments

    Returns:
        list: List of document chunks
    """

    def safe_callback(progress, message):
        if callback:
            callback(progress, message)

    logger.info(f"🔄 MonkeyOCR chunk function started for file: {filename}")
    logger.info(f"📋 Parameters: from_page={from_page}, to_page={to_page}, lang={lang}")
    logger.info(f"📦 Binary size: {len(binary) if binary else 'None'} bytes")
    logger.info(f"⚙️ Parser config: {kwargs.get('parser_config', {})}")

    try:
        safe_callback(0.1, "Starting MonkeyOCR processing with cedd_parse flow...")
        logger.info("✅ Step 1: Starting MonkeyOCR processing")

        # Create MonkeyOCR parser instance
        logger.info("🔧 Creating MonkeyOCR parser instance...")
        parser = MonkeyOCRParser()
        logger.info("✅ MonkeyOCR parser instance created")

        # Save binary to temporary file if needed
        if binary:
            logger.info("💾 Saving binary to temporary file...")
            with tempfile.NamedTemporaryFile(delete=False, suffix=os.path.splitext(filename)[1]) as tmp_file:
                tmp_file.write(binary)
                temp_path = tmp_file.name
            logger.info(f"✅ Binary saved to temporary file: {temp_path}")
        else:
            temp_path = filename
            logger.info(f"📁 Using existing file path: {temp_path}")

        safe_callback(0.2, "Validating file format...")
        logger.info("🔍 Step 2: Validating file format...")

        # Validate file format
        if not parser.validate_file(temp_path):
            error_msg = f"Unsupported file format: {filename}"
            logger.error(f"❌ {error_msg}")
            safe_callback(-1, error_msg)
            return []
        
        logger.info("✅ File format validation passed")

        safe_callback(0.3, "Processing document with cedd_parse full mode...")
        logger.info("🚀 Step 3: Processing document with cedd_parse full mode...")

        # Get parser configuration from kwargs
        parser_config = kwargs.get("parser_config", {})
        logger.info(f"⚙️ Parser config: {parser_config}")
        
        # Use layout_recognize field to determine processing mode
        layout_recognize = parser_config.get("layout_recognize", "MonkeyOCR")
        logger.info(f"🎯 Layout recognize mode: {layout_recognize}")
        
        # Parse document using cedd_parse
        logger.info("📄 Calling parser.parse_document...")
        result = parser.parse_document(temp_path)
        logger.info(f"📊 Parse result success: {result.get('success', False)}")
        
        if result.get("success"):
            safe_callback(0.8, "Converting to RAGFlow chunks...")
            logger.info("🔄 Step 4: Converting to RAGFlow chunks...")

            # Convert to RAGFlow format
            try:
                logger.info("📦 Importing rag.nlp modules...")
                from rag.nlp import tokenize, rag_tokenizer
                import re
                logger.info("✅ rag.nlp modules imported successfully")

                content = result.get("content", "")
                logger.info(f"📝 Content length: {len(content)} characters")
                if not content:
                    content = f"MonkeyOCR processed: {filename}"
                    logger.warning("⚠️ No content found, using fallback content")

                # Create RAGFlow chunk
                logger.info("🏗️ Creating RAGFlow chunk structure...")
                doc = {"docnm_kwd": filename, "title_tks": rag_tokenizer.tokenize(re.sub(r"\.[a-zA-Z]+$", "", filename)), "doc_type_kwd": "monkeyocr"}
                logger.info(f"📋 Chunk structure created: {list(doc.keys())}")

                # Tokenize content
                logger.info("🔤 Tokenizing content...")
                eng = lang.lower() == "english"
                logger.info(f"🌐 Language: {lang}, English mode: {eng}")
                tokenize(doc, content, eng)
                logger.info("✅ Content tokenization completed")

                safe_callback(1.0, "MonkeyOCR processing complete")
                logger.info("🎉 MonkeyOCR processing completed successfully")

                # Cleanup temporary file
                if binary and os.path.exists(temp_path):
                    logger.info("🧹 Cleaning up temporary file...")
                    os.unlink(temp_path)
                    logger.info("✅ Temporary file cleaned up")

                logger.info(f"📤 Returning {len([doc])} chunks")
                return [doc]
            except ImportError as e:
                logger.error(f"❌ ImportError in rag.nlp: {e}")
                # Fallback if rag.nlp is not available
                safe_callback(0.9, "Using fallback chunk format...")
                logger.info("🔄 Using fallback chunk format...")

                content = result.get("content", f"MonkeyOCR processed: {filename}")

                # Create simple chunk format
                doc = {"docnm_kwd": filename, "title_tks": [filename.replace(".", " ").split()], "doc_type_kwd": "monkeyocr", "content": content, "content_tks": content.split()}
                logger.info("✅ Fallback chunk format created")

                safe_callback(1.0, "MonkeyOCR processing complete - Fallback mode")
                logger.info("🎉 MonkeyOCR processing completed with fallback mode")

                # Cleanup temporary file
                if binary and os.path.exists(temp_path):
                    logger.info("🧹 Cleaning up temporary file...")
                    os.unlink(temp_path)
                    logger.info("✅ Temporary file cleaned up")

                logger.info(f"📤 Returning {len([doc])} chunks (fallback)")
                return [doc]
        else:
            error_msg = f"MonkeyOCR failed: {result.get('error', 'Unknown error')}"
            logger.error(f"❌ {error_msg}")
            safe_callback(-1, error_msg)
            return []

    except Exception as e:
        error_msg = f"MonkeyOCR processing failed: {str(e)}"
        logger.error(f"❌ {error_msg}")
        logger.exception(f"Exception details for {filename}:")
        safe_callback(-1, error_msg)
        return []
    finally:
        logger.info(f"🏁 MonkeyOCR chunk function finished for file: {filename}")

if __name__ == "__main__":
    import sys

    def dummy(prog=None, msg=""):
        pass

    chunk(sys.argv[1], from_page=0, to_page=10, callback=dummy)