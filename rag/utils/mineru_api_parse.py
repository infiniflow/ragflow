import requests
import time
import json
import uuid
import yaml
import zipfile
import io
import os
import hashlib
from copy import deepcopy
from pathlib import Path
from dataclasses import dataclass
from enum import Enum
from typing import Dict, List, Any, Optional, Tuple
# from appconf import logger
import logging
import base64

import os
import uuid
from io import BytesIO
from typing import Optional
# 创建一个日志对象，名称可以自定义（通常用当前模块名）
import sys
logger = logging.getLogger(__name__)
project_root = os.path.abspath(os.path.dirname(__file__))
sys.path.insert(0, project_root)

@dataclass
class CreateTaskInfo:
    """
    Model for task creation information
    """
    
    # Unique identifier for the task
    task_id: str
    # URL of the file to be processed
    file_url: str


class TaskProcessingStatus(Enum):
    """
    Enum for task processing status
    """
    DONE = "done"
    WAITING_FILE = "waiting-file"
    PENDING = "pending"
    RUNNING = "running"
    CONVERTING = "converting"
    FAILED = "failed"

@dataclass
class TaskProcessingResult:
    """
    Model for task processing results
    """
    # Status of the task (e.g., 'done', 'failed')
    status: TaskProcessingStatus
    # full zip url parsed
    full_zip_url: str
    err_msg: str


def calculate_file_hash(file_path: str,
                       algorithm: str = "sha256",
                       chunk_size: Optional[int] = None) -> Optional[str]:
    """
    Calculate the hash value of a file by reading entire content by default
    
    Parameters:
        file_path: Path to the file
        algorithm: Hash algorithm (md5/sha1/sha224/sha256/sha384/sha512/blake2b/blake2s)
        chunk_size: Optional read chunk size in bytes (None for full content)
        
    Returns:
        str: File hash value (hexadecimal string)
        None: If file doesn't exist or read error occurs
    """
    # Supported hash algorithms mapping
    hash_algorithms = {
        'md5': hashlib.md5,
        'sha1': hashlib.sha1,
        'sha224': hashlib.sha224,
        'sha256': hashlib.sha256,
        'sha384': hashlib.sha384,
        'sha512': hashlib.sha512,
        'blake2b': hashlib.blake2b,
        'blake2s': hashlib.blake2s
    }
    
    # Validate algorithm support
    algorithm = algorithm.lower()
    if algorithm not in hash_algorithms:
        raise ValueError(f"Unsupported hash algorithm: {algorithm}. Supported: {', '.join(hash_algorithms.keys())}")
    
    # Check if file exists
    if not os.path.exists(file_path):
        logger.error(f"File does not exist: {file_path}")
        return None
    
    try:
        hash_obj = hash_algorithms[algorithm]()
        
        with open(file_path, 'rb') as f:
            if chunk_size:
                # Read in chunks if specified
                while chunk := f.read(chunk_size):
                    hash_obj.update(chunk)
            else:
                # Read entire content by default
                hash_obj.update(f.read())
                
        return hash_obj.hexdigest()
        
    except Exception as e:
        logger.error(f"Error calculating file hash: {e}")
        return None


class MineruDocumentParser:
    def __init__(
            self, 
            file_path: str, 
            api_key: Optional[str] = None,
            cache_folder: Optional[str] = None
        ):
        """
        Initialize document parser
        
        Args:
            file_path: Local path to the file to be parsed
            api_key: API key for MinerU service (optional, will read from config if not provided)
            cache_folder: Folder to save parsed zip and unzip files
        """
        if not file_path or not os.path.exists(file_path) or not os.path.isfile(file_path):
            raise ValueError("File path is folder or does not exist")
        if not isinstance(file_path, str):
            raise TypeError("File path must be a string")
        self.file_path = file_path
        
        # if not api_key:
        #     api_key = global_config.get_mineru_config().api_key
        if not api_key:
            raise ValueError("API key cannot be empty. Please provide a valid API key.")
        self.api_key = api_key
        
        if not cache_folder:
            cache_folder = os.path.join(os.getcwd(), "workspace", 'mineru_cache')
        self.cache_folder = cache_folder
        if not os.path.exists(self.cache_folder):
            os.makedirs(self.cache_folder)
        self.api_base_url = "https://mineru.net/api/v4"
        self.task_id: Optional[str] = None
        self.result: Optional[Any] = None

    def get_file_path(self, file_path: str):
        self.file_path = file_path

    def start_parsing(self) -> bool:
        """
        Initiate file parsing task and wait synchronously for completion
            
        Returns:
            bool: Whether parsing was successful
        """
        cache_folder = self._get_cache_folder()
        if os.path.exists(cache_folder) and self._find_content_json_file(result_unzip_folder=cache_folder):
            logger.info(f"Not need repeat parse {self.file_path}, it has parsed in {cache_folder}")
            self.result_unzip_folder = cache_folder
            return True
        create_task_info = self._create_task()
        self._upload_file_for_task(create_task_info)
        while True:
            task_result = self._query_task_process_result(create_task_info)
            if task_result.status == TaskProcessingStatus.DONE:
                self.task_id = create_task_info.task_id
                self.parsed_zip_url = task_result.full_zip_url
                logger.info(f"Parsing completed successfully for {self.file_path}, parsed zip URL: {self.parsed_zip_url}")
                self._download_and_extract_zip(task_result.full_zip_url, cache_folder)
                self.result_unzip_folder = cache_folder
                logger.info(f"Parsed files extracted to {cache_folder} for file {self.file_path} with parse task id {self.task_id}")
                return True
            elif task_result.status == TaskProcessingStatus.FAILED:
                logger.error(f"Parsing failed for {self.file_path}, error: {task_result.err_msg}")
                return False
            else:
                logger.info(f"Task {create_task_info.task_id} is still processing, status: {task_result.status}")
                time.sleep(3)



    def get_content_list(self) -> List:
        """
        Get parsed table results
        
        Returns:
            str: Parsing results where all tables are connected as a single string by newlines
        """
        if not self.result_unzip_folder:
            raise Exception("No parsing results available. Please call start_parsing first.")
        
        result = {}
        parsed_json_results_file_path = self._find_content_json_file(result_unzip_folder=self.result_unzip_folder)
        if not parsed_json_results_file_path:
            raise Exception("Not found content list json file")
        with open(parsed_json_results_file_path, "rb") as f:
            parsed_json_results = json.load(f)
        if not isinstance(parsed_json_results, list):
            raise Exception("Parsed results are not in expected format (list). Please check the parsing results.")
        
        return parsed_json_results

    def get_base64_images(self) -> Dict[str, str]:
        base64_images:Dict[str, str]={}
        def jpg_to_base64(image_path: str) -> str:
            try:
                with open(image_path, "rb") as image_file:
                    # 读取二进制数据并编码为 Base64 字符串
                    base64_bytes = base64.b64encode(image_file.read())
                    base64_str = base64_bytes.decode("utf-8")
                    # 添加数据头（表明图片格式为 JPG）
                    return f"data:image/jpeg;base64,{base64_str}"
            except FileNotFoundError:
                raise ValueError(f"图片文件未找到: {image_path}")
            except Exception as e:
                raise RuntimeError(f"转换失败: {str(e)}")

        result_unzip_images_folder = os.path.join(self.result_unzip_folder, 'images')
        if not result_unzip_images_folder:
            raise Exception("No parsing results available. Please call start_parsing first.")

        for file_name in os.listdir(result_unzip_images_folder):
            file_path = os.path.join(result_unzip_images_folder, file_name)
            if os.path.isdir(file_path):
                continue
        
            lower_file_name = file_name.lower()
            if not (lower_file_name.endswith(".jpg") or lower_file_name.endswith(".jpeg")):
                return {}, f"错误：存在非JPG格式文件 - {file_name}"
        
            file_name_without_ext = os.path.splitext(file_name)[0]
        
            if file_name_without_ext in base64_images:
                return {}, f"错误：存在重复文件名（去除后缀后） - {file_name_without_ext}"
        
            base64_images[file_name_without_ext] = jpg_to_base64(file_path)
    
        return base64_images       
        
    def get_text(self) -> Dict[str, str]:
        """
        Get parsed text content (reserved for future extension)
        
        Returns:
            Dict[str, str]: Parsing results where keys are filenames 
                           and values are text content
        """
        raise NotImplementedError("Text extraction not implemented yet")

    def get_images(self) -> Dict[str, List[Any]]:
        
        
        
        """
        Get parsed image information (reserved for future extension)
        
        Returns:
            Dict[str, List[Any]]: Parsing results where keys are filenames 
                                 and values are lists of image information
        """
        raise NotImplementedError("Image extraction not implemented yet")

    def _create_task(self) -> CreateTaskInfo:
        url = f"{self.api_base_url}/file-urls/batch"
        header = {
            "Content-Type": "application/json",
            "Authorization":f"Bearer {self.api_key}"
        }
        data_id = str(uuid.uuid4())
        data = {
            "enable_formula": False,
            "language": "auto",
            "enable_table": True,
            "files": [
                {"name":self.file_path, "is_ocr": True, "data_id": data_id}
            ],
            "model_version": "v2",
        }
        response = requests.post(url, headers=header, json=data)
        if response.status_code == 200:
            result = response.json()
            logger.info('MinerU create task response success. result:{}'.format(result))
            if result["code"] == 0:
                batch_id = result["data"]["batch_id"]
                file_url = result["data"]["file_urls"][0]
                return CreateTaskInfo(task_id=batch_id, file_url=file_url)
            else:
                logger.error('MinerU submit task failed, reason:{}'.format(result.get('msg', 'Unknown error')))
                raise RuntimeError('MinerU submit task failed, reason:{}'.format(result.get('msg', 'Unknown error')))
        else:
            logger.error('MinerU response not success. status:{} ,result:{}'.format(response.status_code, response))
            raise RuntimeError('MinerU response not success. status:{} ,result:{}'.format(response.status_code, response))
        
    def _upload_file_for_task(self, create_task_info: CreateTaskInfo) -> None:
        """
        Upload file to MinerU for processing
        
        Args:
            create_task_info: Information about the task including file URL
        """
        
        with open(self.file_path, 'rb') as file:
            res_upload = requests.put(create_task_info.file_url, data=file)
            if res_upload.status_code == 200:
                logger.info(f"MinerU upload file succeed for {self.file_path}")
            else:
                logger.error(f"MinerU upload file failed for {self.file_path}, status: {res_upload.status_code}, response: {res_upload.text}")
                raise RuntimeError(f"MinerU upload file failed for {self.file_path}, status: {res_upload.status_code}, response: {res_upload.text}")
            
    def _query_task_process_result(self, create_task_info: CreateTaskInfo) -> TaskProcessingResult:
        """
        Query the status of a parsing task
        
        Args:
            create_task_info: Information about the task including task ID
            
        Returns:
            Dict[str, Any]: Task status information
        """
        url = f'{self.api_base_url}/extract-results/batch/{create_task_info.task_id}'
        header = {
            "Content-Type":"application/json",
            "Authorization":f"Bearer {self.api_key}"
        }
        res = requests.get(url, headers=header)
        if res.status_code != 200:
            logger.error(f"MinerU query task status failed, status: {res.status_code}, response: {res.text}")
            return TaskProcessingResult(status=TaskProcessingStatus.RUNNING, full_zip_url="", err_msg="")
        res_json_data = res.json()["data"]
        status = TaskProcessingStatus(res_json_data["extract_result"][0]["state"])
        return TaskProcessingResult(
            status=status,
            full_zip_url=res_json_data["extract_result"][0].get("full_zip_url", ""),
            err_msg=res_json_data["extract_result"][0].get("err_msg", "")
        )

    def _download_and_extract_zip(self, zip_url, unzip_folder_name: str) -> str:
        """
        Download and extract a ZIP file from a given URL
        
        Args:
            zip_url (str): online zip url
            unzip_folder_name (str): unzip folder name

        Returns:
            str: Path to the extracted folder
        """
        try:
            response = requests.get(zip_url)
            response.raise_for_status()
            extract_to = os.path.join(self.cache_folder, unzip_folder_name)
            with zipfile.ZipFile(io.BytesIO(response.content)) as zip_ref:
                if not os.path.exists(extract_to):
                    os.makedirs(extract_to)
                zip_ref.extractall(extract_to)
            
        except requests.exceptions.RequestException as e:
            logger.error(f"Error downloading ZIP file: {e}")
            raise
        except zipfile.BadZipFile as e:
            logger.error(f"Invalid ZIP file: {e}")
            raise
        except Exception as e:
            logger.error(f"Error extracting ZIP file: {e}")
            raise
        
    def _get_cache_folder(self):
        file_hash = calculate_file_hash(file_path=self.file_path)
        if not file_hash:
            raise Exception(f"File {self.file_path} is not exist")
        return os.path.join(self.cache_folder, file_hash)

    def _find_content_json_file(self, result_unzip_folder: str):
        for entry in os.scandir(result_unzip_folder):
            if entry.is_file() and entry.name.endswith('_content_list.json'):
                return entry.path
        return None
    


    # def save_pdf_with_unique_id(self, pdf_bytesio: BytesIO, files_dir: str = "pdf_files") -> Optional[str]:

    #     try:
    #         current_file_path = os.path.abspath(__file__)
    #         project_root = os.path.dirname(os.path.dirname(os.path.dirname(current_file_path)))
    #         save_dir = os.path.join(project_root, files_dir)
    #         # 确保保存目录存在
    #         os.makedirs(save_dir, exist_ok=True)
    #         # 生成唯一ID作为文件名（UUID4格式，确保唯一性）
    #         unique_id = str(uuid.uuid4())
    #         file_name = f"{unique_id}.pdf"
    #         file_path = os.path.join(save_dir, file_name)
        
    #         # 检查文件是否已存在（理论上UUID碰撞概率极低，但仍做检查）
    #         if os.path.exists(file_path):
    #             # 若意外存在，重新生成一个ID
    #             unique_id = str(uuid.uuid4())
    #             file_name = f"{unique_id}.pdf"
    #             file_path = os.path.join(save_dir, file_name)
        
    #         # 将BytesIO中的数据写入文件
    #         with open(file_path, "wb") as pdf_file:
    #             pdf_bytesio.seek(0)  # 确保指针在起始位置
    #             pdf_file.write(pdf_bytesio.read())
        
    #         # 验证文件是否有效
    #         if os.path.getsize(file_path) > 0:
    #             return file_path
    #         else:
    #             print(f"错误：生成的PDF文件为空 - {file_path}")
    #             if os.path.exists(file_path):
    #                 os.remove(file_path)  # 清理空文件
    #             return None
            
    #     except Exception as e:
    #         print(f"保存PDF文件失败：{str(e)}")
    #         return None
    
# Usage example
# mineru_api_parser = MineruDocumentParser(api_key="eyJ0eXBlIjoiSldUIiwiYWxnIjoiSFM1MTIifQ.eyJqdGkiOiI0MzMwMDIwMiIsInJvbCI6IlJPTEVfUkVHSVNURVIiLCJpc3MiOiJPcGVuWExhYiIsImlhdCI6MTc1MzkzMTg3MiwiY2xpZW50SWQiOiJsa3pkeDU3bnZ5MjJqa3BxOXgydyIsInBob25lIjoiIiwib3BlbklkIjpudWxsLCJ1dWlkIjoiMDA3MGY5NGItOGVhOC00YzE2LTg4ZmQtYzdmZTUyYzg5NzBhIiwiZW1haWwiOiIiLCJleHAiOjE3NTUxNDE0NzJ9.dBrbXhNjH-qC4cvCpXOufo3I_PxAcApRGrsRmHuH8BTWClGWM08SVHzntCKFGeA94V8jCggR9C7MFU1vmZrx1w")
    # try:
    #     # 1. Initialize parser
    #     parser = MineruDocumentParser(api_key="eyJ0eXBlIjoiSldUIiwiYWxnIjoiSFM1MTIifQ.eyJqdGkiOiI0MzMwMDIwMiIsInJvbCI6IlJPTEVfUkVHSVNURVIiLCJpc3MiOiJPcGVuWExhYiIsImlhdCI6MTc1MzkzMTg3MiwiY2xpZW50SWQiOiJsa3pkeDU3bnZ5MjJqa3BxOXgydyIsInBob25lIjoiIiwib3BlbklkIjpudWxsLCJ1dWlkIjoiMDA3MGY5NGItOGVhOC00YzE2LTg4ZmQtYzdmZTUyYzg5NzBhIiwiZW1haWwiOiIiLCJleHAiOjE3NTUxNDE0NzJ9.dBrbXhNjH-qC4cvCpXOufo3I_PxAcApRGrsRmHuH8BTWClGWM08SVHzntCKFGeA94V8jCggR9C7MFU1vmZrx1w")
    #     # result_unzip_folder, result_filenam_id = parser._download_and_extract_zip("https://cdn-mineru.openxlab.org.cn/pdf/e08aa453-62e6-414c-9a20-7af6290c48fa.zip")
    #     # parser.result_unzip_folder = result_unzip_folder
    #     # parser.result_filenam_id = result_filenam_id
    #     parser.get_file_path("./111.pptx")
    #     # 2. Start parsing
    #     if parser.start_parsing():
    #         print(f"Parse file succeed") 
    #     else:
    #         print(f"Parse file failed")    
    #     mineru_parse_result: dict = {} 
    #     content_list = parser.get_content_list()
    #     # print(f"Parsed tables: {table_info}")  

    #     mineru_parse_result["content_list"] = content_list
    #     print(content_list) 
    #     print(type(content_list)) 
    # except Exception as e:
    #     print(f"Error occurred: {str(e)}")