import os
import io
import math
import re
import json
import base64
import uuid
import asyncio
from io import BytesIO

import logging
import requests
from json_repair import repair_json
from openai import AsyncOpenAI
from PIL import Image

from rag.utils.storage_factory import STORAGE_IMPL
from rag import settings
from rag.utils.mineru_api_parse import MineruDocumentParser
from typing import Optional

MINIO_BUCKET_NAME = "parsed-file-images"
MINIO_ERROR_BUCKET_NAME = "parsed-file-images-error"
BASE_UNIT_DIM = 28
TOKEN_LIMIT = 3500

def convert_ppt_to_txts(ppt_stream: BytesIO):
    """
    Converts a PowerPoint file to text.
    
    Args:
        ppt_stream (BytesIO): PowerPoint file as binary stream
        
    Returns:
        list[str]: List of markdown pages extracted from the presentation
    """
    mineru_url = os.getenv("MINERU_URL")
    print(mineru_url)
    vl_url = os.getenv("VL_URL")
    vl_api_key = os.getenv("VL_API_KEY")
    vl_model = os.getenv("VL_MODEL")
    if mineru_url:
        return extract_using_mineru(ppt_stream, mineru_url, vl_url, vl_api_key, vl_model)
    else:
        return []


def parse_pptx_with_mineru(
        ppt_stream: BytesIO,
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
        ppt_stream (BytesIO): PowerPoint file as binary stream
        mineru_url (str, optional): Mineru API URL
        parse_method (str, optional): Parsing method to use. Defaults to "auto"
        is_json_md_dump (bool, optional): Whether to return JSON markdown dump. Defaults to False
        return_layout (bool, optional): Whether to return layout information. Defaults to False
        return_info (bool, optional): Whether to return additional info. Defaults to False
        return_content_list (bool, optional): Whether to return content list. Defaults to True
        return_images (bool, optional): Whether to return images. Defaults to True
        
    Returns:
        dict: Parsed content from the PowerPoint file
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
        files = {"file": (f"presentation_{uuid.uuid4()}.pptx", ppt_stream,
                          "application/vnd.openxmlformats-officedocument.presentationml.presentation")}
        response = requests.post(url, params=params, files=files)
        response.raise_for_status()
        return response.json()
    except requests.exceptions.RequestException as e:
        logging.error(f"API request error: {e}")
        return None
    except Exception as e:
        logging.error(f"Unknown error occurred: {e}")
        return None


def extract_using_mineru(ppt_stream: BytesIO, mineru_url=None, vl_url=None, vl_api_key=None, vl_model=None):
    """
    Extract text from PowerPoint using Mineru and add image descriptions using vision-language model.
    
    Args:
        ppt_stream (BytesIO): PowerPoint file as binary stream
        mineru_url (str, optional): Mineru API URL
        vl_url (str, optional): Vision-language API URL
        vl_api_key (str, optional): Vision-language API key
        vl_model (str, optional): Vision-language model name
        
    Returns:
        list[str]: List of markdown pages extracted from the presentation
    """
    file_path = save_ppt_with_unique_id(ppt_stream, "ppt_files")
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
    
    # mineru_parse_result = parse_pptx_with_mineru(ppt_stream, mineru_url)

    # content_list = mineru_parse_result["content_list"]
    # markdown_pages = generate_markdown_pages(content_list)
    images = mineru_api_parser.get_base64_images()
    # images = mineru_parse_result["images"]
    if vl_url and vl_api_key and vl_model:
        markdown_pages = add_image_description_to_markdown_pages(markdown_pages, images, vl_url, vl_api_key, vl_model)

    markdown_pages = upload_images(markdown_pages, images)

    return markdown_pages


def add_image_description_to_markdown_pages(markdown_pages, images, vl_url, vl_api_key, vl_model):
    """
    Add image descriptions to markdown pages using vision-language model.
    
    Args:
        markdown_pages (list[str]): List of markdown pages
        images (dict): Dictionary of images with name as key and base64 as value
        vl_url (str): Vision-language API URL
        vl_api_key (str): Vision-language API key
        vl_model (str): Vision-language model name
        
    Returns:
        list[str]: List of markdown pages with image descriptions
    """
    client = AsyncOpenAI(
        api_key=vl_api_key,
        base_url=vl_url,
    )

    vl_results = asyncio.run(describe_images(client, images))
    image_descriptions = dict(vl_results)

    markdown_pages_with_descriptions = replace_image_descriptions(markdown_pages, image_descriptions)

    return markdown_pages_with_descriptions


async def describe_images(client, images):
    """
    Asynchronously describe multiple images using vision-language model.
    
    Args:
        client (AsyncOpenAI): OpenAI client instance
        images (dict): Dictionary of images with name as key and base64 as value
        
    Returns:
        list: List of tuples containing image name and description
    """
    tasks = []
    for image_name, image_base64 in images.items():
        task = asyncio.create_task(describe_image(client, image_name, image_base64))
        tasks.append(task)
    results = await asyncio.gather(*tasks, return_exceptions=True)
    errors = [str(r) for r in results if isinstance(r, Exception)]
    if errors:
        raise Exception(f"Multiple errors occurred:\n" + "\n".join(errors))
    return results


def estimate_qwen_vl_tokens(width: int, height: int) -> int:
    """
    根据通义千问 qwen-vl-plus 模型的规则，估算图像消耗的 tokens。
    规则：基于 28x28 的块，每块为 1 token，不足一块的向上取整。
    """
    if width == 0 or height == 0:
        return 0
    tiles_in_width = math.ceil(width / BASE_UNIT_DIM)
    tiles_in_height = math.ceil(height / BASE_UNIT_DIM)
    return tiles_in_width * tiles_in_height


def resize_image_base64(image_base64: str) -> str:
    """
    将 Base64 编码的 JPG 图像的分辨率缩小至一半。

    Args:
        image_base64 (str): 图像的 Base64 数据 URL (例如, "data:image/jpeg;base64,...").

    Returns:
        str: 缩小后图像的 Base64 数据 URL.
             如果处理失败，则返回原始字符串。
    """
    try:
        header, encoded_data = image_base64.split(",", 1)

        image_data = base64.b64decode(encoded_data)

        image_stream = io.BytesIO(image_data)
        img = Image.open(image_stream)

        if img.mode != 'RGB':
            img = img.convert('RGB')

        current_tokens = estimate_qwen_vl_tokens(img.width, img.height)

        if current_tokens <= TOKEN_LIMIT:
            return image_base64

        target_tokens = TOKEN_LIMIT * 0.98
        scale_factor = math.sqrt(target_tokens / current_tokens)

        new_width = int(img.width * scale_factor)
        new_height = int(img.height * scale_factor)

        resized_img = img.resize((new_width, new_height), Image.Resampling.LANCZOS)

        output_buffer = io.BytesIO()
        resized_img.save(output_buffer, format="JPEG")
        img_bytes = output_buffer.getvalue()

        new_encoded_data = base64.b64encode(img_bytes).decode('utf-8')
        return f"{header},{new_encoded_data}"

    except Exception as e:
        print(f"Warning: Could not resize image. Using original. Error: {e}")
        return image_base64


async def describe_image(client, image_name, image_base64):
    """
    Describe a single image using vision-language model.
    
    Args:
        client (AsyncOpenAI): OpenAI client instance
        image_name (str): Image name
        image_base64 (str): Image base64 string
        
    Returns:
        tuple: Tuple containing image name and description
    """
    vl_prompt_path = os.path.join(os.path.dirname(__file__), "vl_prompt.txt")
    try:
        with open(vl_prompt_path, 'r', encoding='utf-8') as file:
            vl_prompt = file.read()
    except FileNotFoundError:
        print(f"错误：文件 {vl_prompt_path} 未找到")
    except Exception as e:
        print(f"读取文件时出错: {e}")

    resized_image_base64 = resize_image_base64(image_base64)

    try:
        completion = await client.chat.completions.create(
            model=os.getenv('VL_MODEL'),
            messages=[
                {
                    "role": "system",
                    "content": [{"type": "text", "text": "You are a helpful assistant."}]
                },
                {
                    "role": "user",
                    "content": [
                        {
                            "type": "image_url",
                            "image_url": {"url": resized_image_base64},
                        },
                        {"type": "text", "text": vl_prompt},
                    ],
                }
            ],
        )
        return image_name, completion.choices[0].message.content
    except Exception as e:
        _, base64_str = image_base64.split(",", 1)
        upload_images_to_minio(image_name, base64_str, MINIO_ERROR_BUCKET_NAME)
        raise Exception(f"Error processing image {image_name}: {str(e)}") from e


def replace_image_descriptions(markdown_pages, image_descriptions):
    """
    Replace image descriptions in markdown pages.
    
    Args:
        markdown_pages (list[str]): List of markdown pages
        image_descriptions (dict): Dictionary of image descriptions with name as key and description as value
        
    Returns:
        list[str]: List of markdown pages with updated image descriptions
    """
    updated_pages = []

    for page in markdown_pages:
        # Use regex to find all image markers
        image_matches = re.finditer(r'!\[(.*?)\]\((.*?)\s*(?:"(.*?)")?\)', page)

        # Record positions and content that need to be replaced
        replacements = []

        for match in image_matches:
            full_match = match.group(0)
            img_path = match.group(2)

            # Extract image filename
            img_name = img_path.split('/')[-1]

            # If image is in the description dictionary
            if img_name in image_descriptions:
                description_str = image_descriptions[img_name]
                if description_str is None:
                    continue
                description_dict = repair_json(description_str, ensure_ascii=False, return_objects=True)
                title = description_dict.pop("title", "")
                alt_text = json.dumps(description_dict, ensure_ascii=False)

                new_markdown = f'![{alt_text}]({img_path} "{title}")'

                replacements.append((full_match, new_markdown))

        # Perform replacements
        updated_page = page
        for old, new in replacements:
            updated_page = updated_page.replace(old, new)

        updated_pages.append(updated_page)

    return updated_pages


def upload_images(markdown_pages: list[str], images):
    """
    Upload images to MinIO storage and replace image paths in markdown pages.
    
    Args:
        markdown_pages (list[str]): List of markdown pages
        images (dict): Dictionary of images with name as key and base64 as value
        
    Returns:
        list[str]: List of markdown pages with updated image paths
    """
    for image_name, image_base64 in images.items():
        _, base64_str = image_base64.split(",", 1)

        permanent_url = upload_images_to_minio(image_name, base64_str, MINIO_BUCKET_NAME)

        for i, page in enumerate(markdown_pages):
            if f"images/{image_name}" in page:
                markdown_pages[i] = page.replace(f"images/{image_name}", permanent_url)
                break

    return markdown_pages


def upload_images_to_minio(image_name, image_base64, bucket_name):
    """
    Upload a single image to MinIO storage.
    
    Args:
        image_name (str): Image name
        image_base64 (str): Image base64 string
        bucket_name: Minio bucket name
        
    Returns:
        str: Permanent URL for the uploaded image
    """
    policy = {
        "Version": "2012-10-17",
        "Statement": [
            {
                "Effect": "Allow",
                "Principal": "*",
                "Action": ["s3:GetObject"],
                "Resource": [f"arn:aws:s3:::{bucket_name}/*"]
            }
        ]
    }
    policy_str = json.dumps(policy)

    while STORAGE_IMPL.obj_exist(bucket_name, image_name):
        name, ext = os.path.splitext(image_name)
        image_name = f"{name}_{ext}"
    image_base64 = base64.b64decode(image_base64)

    client = STORAGE_IMPL.conn
    if not client.bucket_exists(bucket_name):
        client.make_bucket(bucket_name)
    client.set_bucket_policy(bucket_name, policy_str)
    STORAGE_IMPL.put(bucket_name, image_name, image_base64)
    permanent_url = f"http://{os.getenv('MINIO_BASE_URL')}/{bucket_name}/{image_name}"
    return permanent_url


def generate_markdown_pages(content_list):
    """
    Generate markdown pages from content list.
    
    Args:
        content_list (list): List of content items from parsed PowerPoint
        
    Returns:
        list[str]: List of markdown pages
    """
    # Find all unique page indices
    page_indices = sorted({item['page_idx'] for item in content_list})

    markdown_pages = []

    for page_idx in page_indices:
        page_content = []
        page_items = [item for item in content_list if item['page_idx'] == page_idx]

        for item in page_items:
            if item['type'] == 'text':
                text = item['text'].strip()
                if not text:
                    continue

                text_level = item.get('text_level', 0)
                if text_level == 1:
                    page_content.append(f"# {text}")
                elif text_level == 2:
                    page_content.append(f"## {text}")
                elif text_level == 3:
                    page_content.append(f"### {text}")
                else:
                    page_content.append(text)

            elif item['type'] in ['table', 'image']:
                img_path = item.get('img_path', '')
                if not img_path:
                    continue
                markdown_image = f'![]({img_path})'
                page_content.append(markdown_image)

        # Combine current page content
        markdown_pages.append("\n\n".join(page_content))

    return markdown_pages


def save_ppt_with_unique_id(ppt_bytesio: BytesIO, files_dir: str = "ppt_files") -> Optional[str]:
    """
    将字节流形式的PPT保存为具有唯一ID的本地PPT文件
    
    Args:
        ppt_bytesio: 包含PPT数据的BytesIO对象
        files_dir: 保存PPT文件的目录，默认为"ppt_files"
        
    Returns:
        成功时返回文件完整路径，失败时返回None
    """
    try:
        # 获取当前文件路径并计算项目根目录（根据实际目录结构调整）
        current_file_path = os.path.abspath(__file__)
        project_root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(current_file_path))))
        save_dir = os.path.join(project_root, files_dir)
        
        # 确保保存目录存在
        os.makedirs(save_dir, exist_ok=True)
        
        # 生成唯一ID作为文件名（UUID4格式）
        unique_id = str(uuid.uuid4())
        file_name = f"{unique_id}.ppt"  # 针对.ppt格式
        # 如果需要支持.pptx格式，可以使用：file_name = f"{unique_id}.pptx"
        file_path = os.path.join(save_dir, file_name)
    
        # 检查文件是否已存在（UUID碰撞概率极低，做双重保障）
        if os.path.exists(file_path):
            unique_id = str(uuid.uuid4())
            file_name = f"{unique_id}.ppt"
            file_path = os.path.join(save_dir, file_name)
    
        # 将BytesIO中的PPT数据写入文件
        with open(file_path, "wb") as ppt_file:
            ppt_bytesio.seek(0)  # 确保指针流指针移至起始位置，确保读取完整数据
            ppt_file.write(ppt_bytesio.read())
    
        # 验证文件是否有效（非空）
        if os.path.getsize(file_path) > 0:
            return file_path
        else:
            print(f"错误：生成的PPT文件为空 - {file_path}")
            if os.path.exists(file_path):
                os.remove(file_path)  # 清理空文件
            return None
        
    except Exception as e:
        print(f"保存PPT文件失败：{str(e)}")
        return None