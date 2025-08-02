import os
import uuid
from io import BytesIO

import logging
import requests

from rag.utils.ppt_to_txts import generate_markdown_pages, add_image_description_to_markdown_pages, upload_images
from rag.utils.mineru_api_parse import MineruDocumentParser
from typing import Optional


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
    '''

    '''

    file_path = save_pdf_with_unique_id(pdf_stream, "pdf_files")
    # file_path = mineru_api_parser.save_pdf_with_unique_id(pdf_stream)
    
    mineru_api_parser = MineruDocumentParser(api_key="eyJ0eXBlIjoiSldUIiwiYWxnIjoiSFM1MTIifQ.eyJqdGkiOiI0MzMwMDIwMiIsInJvbCI6IlJPTEVfUkVHSVNURVIiLCJpc3MiOiJPcGVuWExhYiIsImlhdCI6MTc1MzkzMTg3MiwiY2xpZW50SWQiOiJsa3pkeDU3bnZ5MjJqa3BxOXgydyIsInBob25lIjoiIiwib3BlbklkIjpudWxsLCJ1dWlkIjoiMDA3MGY5NGItOGVhOC00YzE2LTg4ZmQtYzdmZTUyYzg5NzBhIiwiZW1haWwiOiIiLCJleHAiOjE3NTUxNDE0NzJ9.dBrbXhNjH-qC4cvCpXOufo3I_PxAcApRGrsRmHuH8BTWClGWM08SVHzntCKFGeA94V8jCggR9C7MFU1vmZrx1w"
                                ,file_path=file_path)
    if mineru_api_parser.start_parsing():
        print(f"Parse file succeed") 
    else:
        print(f"Parse file failed")  
    content_list = mineru_api_parser.get_content_list()

    # mineru_parse_result = parse_pdf_with_mineru(pdf_stream, mineru_url)

    # content_list = mineru_parse_result["content_list"]
    markdown_pages = generate_markdown_pages(content_list)

    # images = mineru_parse_result["images"]
    images = mineru_api_parser.get_base64_images()
    if vl_url and vl_api_key and vl_model:
        markdown_pages = add_image_description_to_markdown_pages(markdown_pages, images, vl_url, vl_api_key, vl_model)

    markdown_pages = upload_images(markdown_pages, images)

    return markdown_pages

def save_pdf_with_unique_id(pdf_bytesio: BytesIO, files_dir: str = "pdf_files") -> Optional[str]:

    try:
        current_file_path = os.path.abspath(__file__)
        project_root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(current_file_path))))
        save_dir = os.path.join(project_root, files_dir)
        # 确保保存目录存在
        os.makedirs(save_dir, exist_ok=True)
        # 生成唯一ID作为文件名（UUID4格式，确保唯一性）
        unique_id = str(uuid.uuid4())
        file_name = f"{unique_id}.pdf"
        file_path = os.path.join(save_dir, file_name)
    
        # 检查文件是否已存在（理论上UUID碰撞概率极低，但仍做检查）
        if os.path.exists(file_path):
            # 若意外存在，重新生成一个ID
            unique_id = str(uuid.uuid4())
            file_name = f"{unique_id}.pdf"
            file_path = os.path.join(save_dir, file_name)
    
        # 将BytesIO中的数据写入文件
        with open(file_path, "wb") as pdf_file:
            pdf_bytesio.seek(0)  # 确保指针在起始位置
            pdf_file.write(pdf_bytesio.read())
    
        # 验证文件是否有效
        if os.path.getsize(file_path) > 0:
            return file_path
        else:
            print(f"错误：生成的PDF文件为空 - {file_path}")
            if os.path.exists(file_path):
                os.remove(file_path)  # 清理空文件
            return None
        
    except Exception as e:
        print(f"保存PDF文件失败：{str(e)}")
        return None
    

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