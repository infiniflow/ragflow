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

import io
import re

import numpy as np
from PIL import Image

from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from deepdoc.vision import OCR
from rag.nlp import tokenize
from rag.utils import clean_markdown_block
from rag.nlp import rag_tokenizer
from rag.prompts import vision_llm_figure_describe_prompt


ocr = OCR()


def _detect_rating_scale_form(ocr_text: str) -> bool:
    """
    Detect if the OCR text suggests a rating scale form or evaluation.
    
    Args:
        ocr_text (str): OCR extracted text from the image
        
    Returns:
        bool: True if rating scale form patterns are detected
    """
    if not ocr_text:
        return False
    
    # Convert to lowercase for pattern matching
    text_lower = ocr_text.lower()
    
    # Pattern indicators for rating scale forms
    rating_indicators = [
        # Scale anchor patterns
        r'\(best\).*\(worst\)',  # (Best) ... (Worst)
        r'\(excellent\).*\(poor\)',  # (Excellent) ... (Poor)
        r'markings.*best.*worst',  # "Markings (Best) 1 2 3 4 5 (Worst)"
        
        # Form structure patterns  
        r'evaluation|assessment|rating|survey',
        r'awards|performance|score',
        r'site.*management|contractor|construction',
        
        # Circle/selection patterns
        r'[○●]{3,}',  # Multiple circles in sequence
        r'circle.*select|mark.*circle',  # Instructions about circles
        
        # Numbered item patterns with circles
        r'\d+\.\s*.{10,80}[○●]',  # Numbered items followed by circles
        
        # Scale number patterns
        r'\b[1-5]\s+[1-5]\s+[1-5]',  # Sequential numbers like "1 2 3 4 5"
    ]
    
    # Chinese rating form indicators
    chinese_indicators = [
        '评分', '评价', '调查', '问卷',  # rating, evaluation, survey, questionnaire
        '表现', '管理', '安全',  # performance, management, safety
        '工地', '承包商',  # construction site, contractor
    ]
    
    # Count pattern matches
    pattern_count = 0
    
    # Check English patterns
    for pattern in rating_indicators:
        if re.search(pattern, text_lower):
            pattern_count += 1
    
    # Check Chinese patterns
    for indicator in chinese_indicators:
        if indicator in ocr_text:
            pattern_count += 1
    
    # Check for high circle density (indicates form with many rating options)
    circles = re.findall(r'[○●]', ocr_text)
    if len(circles) >= 15:  # Many circles suggest a rating form
        pattern_count += 2
    
    # Check for table-like structure with numbers
    table_pattern = r'\d+\.\s*.{5,50}\s+[1-5]\s+[1-5]'
    if re.search(table_pattern, ocr_text):
        pattern_count += 1
    
    # Require at least 2 indicators for positive detection
    return pattern_count >= 2


def chunk(filename, binary, tenant_id, lang, callback=None, **kwargs):
    img = Image.open(io.BytesIO(binary)).convert('RGB')
    doc = {
        "docnm_kwd": filename,
        "title_tks": rag_tokenizer.tokenize(re.sub(r"\.[a-zA-Z]+$", "", filename)),
        "image": img,
        "doc_type_kwd": "image"
    }
    bxs = ocr(np.array(img))
    txt = "\n".join([t[0] for _, t in bxs if t[0]])
    eng = lang.lower() == "english"
    callback(0.4, "Finish OCR: (%s ...)" % txt[:12])
    if (eng and len(txt.split()) > 32) or len(txt) > 32:
        tokenize(doc, txt, eng)
        callback(0.8, "OCR results is too long to use CV LLM.")
        return [doc]

    try:
        callback(0.4, "Use CV LLM to describe the picture.")
        cv_mdl = LLMBundle(tenant_id, LLMType.IMAGE2TEXT, lang=lang)
        img_binary = io.BytesIO()
        img.save(img_binary, format='JPEG')
        img_binary.seek(0)
        
        # Check if this might be a rating scale form
        if _detect_rating_scale_form(txt):
            callback(0.6, "Detected rating scale form, using specialized analysis.")
            # Use the enhanced figure description prompt which now handles rating forms
            ans = cv_mdl.describe_with_prompt(img_binary.read(), vision_llm_figure_describe_prompt())
        else:
            ans = cv_mdl.describe(img_binary.read())
            
        callback(0.8, "CV LLM respond: %s ..." % ans[:32])
        txt += "\n" + ans
        tokenize(doc, txt, eng)
        return [doc]
    except Exception as e:
        callback(prog=-1, msg=str(e))

    return []


def vision_llm_chunk(binary, vision_model, prompt=None, callback=None):
    """
    A wrapper to process image to markdown texts via VLM.
    Enhanced with automatic detection for rating scale forms.

    Returns:
        Simple markdown texts generated by VLM.
    """
    callback = callback or (lambda prog, msg: None)

    img = binary
    txt = ""

    try:
        img_binary = io.BytesIO()
        img.save(img_binary, format='JPEG')
        img_binary.seek(0)

        # If no custom prompt provided, try to detect content type
        if prompt is None:
            try:
                # Quick OCR check to detect content type
                img_array = np.array(img)
                bxs = ocr(img_array)
                ocr_text = "\n".join([t[0] for _, t in bxs if t[0]])
                
                if _detect_rating_scale_form(ocr_text):
                    callback(0.2, "Rating scale form detected, using enhanced analysis")
                    prompt = vision_llm_figure_describe_prompt()
                else:
                    callback(0.2, "Using standard vision analysis")
                    # Let the vision model use its default prompt
                    
            except Exception as ocr_e:
                callback(0.2, f"OCR detection failed: {str(ocr_e)}, using standard prompt")
                # Continue with default prompt if OCR fails

        ans = clean_markdown_block(vision_model.describe_with_prompt(img_binary.read(), prompt))

        txt += "\n" + ans

        return txt

    except Exception as e:
        callback(-1, str(e))

    return ""


def vision_llm_chunk_with_rating_analysis(binary, vision_model, callback=None):
    """
    Convenience function to explicitly use rating scale form analysis.
    Use this when you know the image contains evaluation forms or rating scales.
    
    Args:
        binary: PIL Image object
        vision_model: Vision model instance (LLMBundle)
        callback: Optional progress callback function
        
    Returns:
        str: Markdown text with specialized rating form analysis
    """
    return vision_llm_chunk(
        binary=binary,
        vision_model=vision_model,
        prompt=vision_llm_figure_describe_prompt(),
        callback=callback
    )
