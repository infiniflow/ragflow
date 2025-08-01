import os
import uuid
from io import BytesIO

import logging
import requests

from rag.utils.ppt_to_txts import generate_markdown_pages, add_image_description_to_markdown_pages, upload_images


def convert_pdf_to_txts(pdf_stream: BytesIO):
    """
    Converts a PowerPoint file to text.
    
    Args:
        pdf_stream (BytesIO): PDF file as binary stream
        
    Returns:
        list[str]: List of markdown pages extracted from the pdf
    """
    mineru_url = os.getenv("MINERU_URL")
    vl_url = os.getenv("VL_URL")
    vl_api_key = os.getenv("VL_API_KEY")
    vl_model = os.getenv("VL_MODEL")
    if mineru_url:
        return extract_using_mineru(pdf_stream, mineru_url, vl_url, vl_api_key, vl_model)
    else:
        return []


def extract_using_mineru(pdf_stream: BytesIO, mineru_url=None, vl_url=None, vl_api_key=None, vl_model=None):
    """
    Extract text from PowerPoint using Mineru and add image descriptions using vision-language model.
    
    Args:
        pdf_stream (BytesIO): PDF file as binary stream
        mineru_url (str, optional): Mineru API URL
        vl_url (str, optional): Vision-language API URL
        vl_api_key (str, optional): Vision-language API key
        vl_model (str, optional): Vision-language model name
        
    Returns:
        list[str]: List of markdown pages extracted from the pdf
    """
    mineru_parse_result = parse_pdf_with_mineru(pdf_stream, mineru_url)

    content_list = mineru_parse_result["content_list"]
    markdown_pages = generate_markdown_pages(content_list)

    images = mineru_parse_result["images"]
    if vl_url and vl_api_key and vl_model:
        markdown_pages = add_image_description_to_markdown_pages(markdown_pages, images, vl_url, vl_api_key, vl_model)

    markdown_pages = upload_images(markdown_pages, images)

    return markdown_pages


def parse_pdf_with_mineru(
        pdf_stream: BytesIO,
        mineru_url=None,
        parse_method="auto",
        is_json_md_dump=False,
        return_layout=False,
        return_info=False,
        return_content_list=True,
        return_images=True
):
    """
    Parse PowerPoint file using Mineru API.

    Args:
        pdf_stream (BytesIO): PDF file as binary stream
        mineru_url (str, optional): Mineru API URL
        parse_method (str, optional): Parsing method to use. Defaults to "auto"
        is_json_md_dump (bool, optional): Whether to return JSON markdown dump. Defaults to False
        return_layout (bool, optional): Whether to return layout information. Defaults to False
        return_info (bool, optional): Whether to return additional info. Defaults to False
        return_content_list (bool, optional): Whether to return content list. Defaults to True
        return_images (bool, optional): Whether to return images. Defaults to True

    Returns:
        dict: Parsed content from the PDF file
    """
    url = mineru_url + "/file_parse"

    # Prepare request parameters
    params = {
        "parse_method": parse_method,
        "is_json_md_dump": is_json_md_dump,
        "return_layout": return_layout,
        "return_info": return_info,
        "return_content_list": return_content_list,
        "return_images": return_images
    }

    # Prepare file object
    try:
        files = {"file": (f"presentation_{uuid.uuid4()}.pdf", pdf_stream,  "application/pdf")}
        response = requests.post(url, params=params, files=files)
        response.raise_for_status()
        return response.json()
    except requests.exceptions.RequestException as e:
        logging.error(f"API request error: {e}")
        return None
    except Exception as e:
        logging.error(f"Unknown error occurred: {e}")
        return None