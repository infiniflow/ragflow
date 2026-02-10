import requests
import json
import time
import os
from pathlib import Path
from typing import Dict, Any, Optional, List, Union
import zipfile
import io

class MinerUFileParser:
    """MinerU文件解析客户端 - 针对/file_parse接口"""
    
    def __init__(self, base_url: str = "http://localhost:8000", api_key: Optional[str] = None):
        """
        初始化客户端
        
        参数:
            base_url: MinerU服务地址，默认为本地8080端口
            api_key: API密钥（如果需要认证）
        """
        self.base_url = base_url.rstrip('/')
        self.api_key = api_key
        self.session = requests.Session()
        
        # 设置请求头
        self.session.headers.update({
            'Accept': 'application/json',
            'User-Agent': 'MinerU-File-Parser/1.0'
        })
        
        if api_key:
            self.session.headers.update({'Authorization': f'Bearer {api_key}'})
    
    def parse_files(self, 
                   files: List[str],
                   output_dir: Optional[str] = None,
                   lang_list: Optional[List[str]] = None,
                   backend: str = "pipeline",
                   parse_method: str = "auto",
                   formula_enable: bool = True,
                   table_enable: bool = True,
                   server_url: Optional[str] = None,
                   return_md: bool = True,
                   return_middle_json: bool = False,
                   return_model_output: bool = False,
                   return_content_list: bool = True,
                   return_images: bool = False,
                   response_format_zip: bool = False,
                   start_page_id: int = 0,
                   end_page_id: Optional[int] = None) -> Dict[str, Any]:
        """
        解析PDF或图像文件
        
        参数:
            files: 要解析的文件路径列表
            output_dir: 输出目录（服务器端）
            lang_list: 语言列表，提高OCR准确率
            backend: 解析后端，可选：pipeline, vlm-auto-engine, vlm-http-client, hybrid-auto-engine, hybrid-http-client
            parse_method: 解析方法，可选：auto, txt, ocr
            formula_enable: 是否启用公式解析
            table_enable: 是否启用表格解析
            server_url: 适用于vlm/hybrid-http-client后端的OpenAI兼容服务器URL
            return_md: 是否在响应中返回markdown内容
            return_middle_json: 是否返回中间JSON
            return_model_output: 是否返回模型输出JSON
            return_content_list: 是否返回内容列表JSON
            return_images: 是否返回提取的图像
            response_format_zip: 是否以ZIP文件格式返回结果
            start_page_id: 起始页码（从0开始）
            end_page_id: 结束页码（从0开始）
        
        返回:
            解析结果的字典
        """
        # 构建请求URL
        parse_url = f"{self.base_url}/file_parse"
        
        # 准备文件数据
        file_objs = []
        for file_path in files:
            if not os.path.exists(file_path):
                raise FileNotFoundError(f"文件不存在: {file_path}")
            
            file_name = os.path.basename(file_path)
            mime_type = self._get_mime_type(file_path)
            
            # 先读取文件内容，避免文件对象在请求发送前关闭
            with open(file_path, 'rb') as f:
                file_content = f.read()
            
            file_objs.append(('files', (file_name, file_content, mime_type)))
        
        # 准备表单数据
        form_data = {
            'backend': backend,
            'parse_method': parse_method,
            'formula_enable': str(formula_enable).lower(),
            'table_enable': str(table_enable).lower(),
            'return_md': str(return_md).lower(),
            'return_middle_json': str(return_middle_json).lower(),
            'return_model_output': str(return_model_output).lower(),
            'return_content_list': str(return_content_list).lower(),
            'return_images': str(return_images).lower(),
            'response_format_zip': str(response_format_zip).lower(),
            'start_page_id': str(start_page_id)
        }
        
        # 添加可选参数
        if output_dir:
            form_data['output_dir'] = output_dir
        
        if end_page_id is not None:
            form_data['end_page_id'] = str(end_page_id)
        
        if server_url:
            form_data['server_url'] = server_url
        
        # 处理语言列表
        if lang_list:
            # 将语言列表转换为JSON字符串
            form_data['lang_list'] = json.dumps(lang_list)
        
        try:
            print(f"正在解析 {len(files)} 个文件...")
            print(f"使用的后端: {backend}")
            print(f"解析方法: {parse_method}")
            
            response = self.session.post(
                parse_url,
                files=file_objs,
                data=form_data,
                timeout=300  # 设置较长的超时时间，因为解析可能需要时间
            )
            
            print(f"状态码: {response.status_code}")
            
            # 即使状态码不是200，也尝试处理响应
            if response.status_code == 200:
                # 根据响应格式处理结果
                if response_format_zip:
                    return self._handle_zip_response(response)
                else:
                    return self._handle_json_response(response)
            else:
                # 处理非200状态码
                error_msg = f"服务器返回错误状态码: {response.status_code}"
                print(f"❌ {error_msg}")
                
                try:
                    error_data = response.json()
                    if 'error' in error_data:
                        error_msg = f"服务器错误: {error_data['error']}"
                        print(f"❌ {error_msg}")
                        
                        # 提供MinerU特定错误的解决方案
                        if "HuggingFace Hub" in error_msg:
                            print("\n💡 可能的解决方案:")
                            print("1. 检查MinerU容器是否有网络连接")
                            print("2. 确保容器可以访问huggingface.co")
                            print("3. 考虑使用预下载的模型")
                            print("4. 尝试设置HF_ENDPOINT环境变量")
                            
                except json.JSONDecodeError:
                    error_content = response.text[:500]
                    print(f"❌ 错误响应内容: {error_content}")
                    
                # 返回错误信息而不是抛出异常
                return {
                    'status': 'error',
                    'status_code': response.status_code,
                    'message': error_msg,
                    'response': response.text
                }
                
        except requests.exceptions.Timeout:
            error_msg = "请求超时，解析可能需要更长时间"
            print(f"❌ {error_msg}")
            return {
                'status': 'error',
                'message': error_msg
            }
        except requests.exceptions.RequestException as e:
            error_msg = f"请求失败: {str(e)}"
            print(f"❌ {error_msg}")
            if hasattr(e, 'response') and e.response is not None:
                print(f"错误响应: {e.response.text[:500]}")
            return {
                'status': 'error',
                'message': error_msg
            }
        except Exception as e:
            error_msg = f"解析过程中发生错误: {str(e)}"
            print(f"❌ {error_msg}")
            return {
                'status': 'error',
                'message': error_msg
            }
    
    def _get_mime_type(self, file_path: str) -> str:
        """根据文件扩展名获取MIME类型"""
        ext = os.path.splitext(file_path)[1].lower()
        mime_types = {
            '.pdf': 'application/pdf',
            '.png': 'image/png',
            '.jpg': 'image/jpeg',
            '.jpeg': 'image/jpeg',
            '.tiff': 'image/tiff',
            '.bmp': 'image/bmp',
            '.gif': 'image/gif'
        }
        return mime_types.get(ext, 'application/octet-stream')
    
    def _handle_json_response(self, response: requests.Response) -> Dict[str, Any]:
        """处理JSON格式的响应"""
        try:
            result = response.json()
            print("✅ 解析成功")
            return result
        except json.JSONDecodeError as e:
            print(f"❌ JSON解析失败: {e}")
            print(f"响应内容前500字符: {response.text[:500]}")
            raise
    
    def _handle_zip_response(self, response: requests.Response) -> Dict[str, Any]:
        """处理ZIP格式的响应"""
        content_type = response.headers.get('content-type', '')
        
        if 'application/zip' in content_type or 'application/x-zip-compressed' in content_type:
            print("📦 收到ZIP格式响应")
            
            # 保存ZIP文件
            timestamp = int(time.time())
            zip_filename = f"mineru_results_{timestamp}.zip"
            
            with open(zip_filename, 'wb') as f:
                f.write(response.content)
            
            print(f"ZIP文件已保存: {zip_filename}")
            
            # 解压并读取内容
            result = {'zip_file': zip_filename, 'extracted_files': []}
            
            with zipfile.ZipFile(zip_filename, 'r') as zip_ref:
                # 列出所有文件
                file_list = zip_ref.namelist()
                print(f"ZIP中包含 {len(file_list)} 个文件:")
                
                for file_name in file_list:
                    result['extracted_files'].append(file_name)
                    print(f"  - {file_name}")
                    
                    # 如果是JSON文件，可以读取内容
                    if file_name.endswith('.json'):
                        with zip_ref.open(file_name) as f:
                            try:
                                content = json.loads(f.read().decode('utf-8'))
                                result[file_name] = content
                            except:
                                pass
            
            return result
        else:
            print("⚠️  预期ZIP格式但收到其他格式")
            return self._handle_json_response(response)
    
    def test_connection(self) -> bool:
        """测试连接是否正常"""
        try:
            health_url = f"{self.base_url}/docs"
            response = self.session.get(health_url, timeout=10)
            return response.status_code == 200
        except:
            return False
    
    def save_results(self, results: Dict[str, Any], output_dir: str = "results"):
        """保存解析结果"""
        # 创建输出目录
        os.makedirs(output_dir, exist_ok=True)
        
        timestamp = int(time.time())
        
        # 保存主要结果
        if isinstance(results, dict):
            # 保存为JSON
            json_file = os.path.join(output_dir, f"mineru_result_{timestamp}.json")
            with open(json_file, 'w', encoding='utf-8') as f:
                json.dump(results, f, indent=2, ensure_ascii=False)
            print(f"结果已保存到: {json_file}")
            
            # 如果有markdown内容，单独保存
            if 'markdown' in results:
                md_file = os.path.join(output_dir, f"mineru_result_{timestamp}.md")
                with open(md_file, 'w', encoding='utf-8') as f:
                    f.write(results['markdown'])
                print(f"Markdown已保存到: {md_file}")
            
            # 保存内容列表
            if 'content_list' in results and isinstance(results['content_list'], list):
                content_file = os.path.join(output_dir, f"mineru_content_{timestamp}.json")
                with open(content_file, 'w', encoding='utf-8') as f:
                    json.dump(results['content_list'], f, indent=2, ensure_ascii=False)
                print(f"内容列表已保存到: {content_file}")


# ==================== 使用示例 ====================

if __name__ == "__main__":
    # 1. 初始化客户端
    parser = MinerUFileParser(
        base_url="http://localhost:8000",  # 替换为你的MinerU服务地址
        api_key=None  # 如果需要认证
    )
    
    # 2. 测试连接
    if not parser.test_connection():
        print("❌ 无法连接到MinerU服务，请检查服务是否运行")
        exit(1)
    
    print("✅ 成功连接到MinerU服务")
    
    # 3. 准备要解析的文件
    # 替换为你的文件路径
    files_to_parse = [
        "自然资源统一调查监测现状图建设的若干探索_韩爱惠.pdf",
        # "/path/to/your/document2.pdf",  # 可以同时解析多个文件
    ]
    
    # 检查文件是否存在
    for file_path in files_to_parse:
        if not os.path.exists(file_path):
            print(f"❌ 文件不存在: {file_path}")
            print("请创建测试文件或使用示例文本模式")
            # 使用示例模式
            files_to_parse = []
            break
    
    # 4. 配置解析参数
    try:
        # # 示例1: 使用pipeline后端（通用，支持多语言）
        # print("\n" + "="*60)
        # print("示例1: 使用pipeline后端解析")
        # print("="*60)
        
        # results1 = parser.parse_files(
        #     files=files_to_parse if files_to_parse else [],  # 如果没文件，会使用默认测试
        #     output_dir="/tmp/mineru_output",  # 服务器端输出目录
        #     lang_list=["ch"],  # 中文、英文、繁体中文
        #     backend="pipeline",
        #     parse_method="auto",  # 自动选择解析方法
        #     formula_enable=True,
        #     table_enable=True,
        #     return_md=True,
        #     return_content_list=True,
        #     response_format_zip=False,  # 设置为True可获取ZIP格式结果
        #     start_page_id=0,
        #     end_page_id=10  # 只解析前10页
        # )
        
        # # 保存结果
        # parser.save_results(results1, "results/pipeline_example")
        
        # # 显示结果摘要
        # print("\n📊 解析结果摘要:")
        # if isinstance(results1, dict):
        #     for key, value in results1.items():
        #         if isinstance(value, (str, int, float, bool)):
        #             print(f"  {key}: {value}")
        #         elif isinstance(value, list):
        #             print(f"  {key}: 列表，包含 {len(value)} 个元素")
        #         elif isinstance(value, dict):
        #             print(f"  {key}: 字典，包含 {len(value)} 个键")
        
        # 示例2: 使用vlm-http-client后端（需要OpenAI兼容服务器）
        # print("\n" + "="*60)
        # print("示例2: 使用vlm-http-client后端解析")
        # print("="*60)
        
        # # 注意：这个后端需要提供server_url
        # results2 = parser.parse_files(
        #     files=files_to_parse if files_to_parse else [],
        #     backend="vlm-http-client",
        #     server_url="http://127.0.0.1:30000",  # 你的OpenAI兼容服务器地址
        #     lang_list=["ch"],
        #     formula_enable=True,
        #     table_enable=True,
        #     return_md=True
        # )
        
        # parser.save_results(results2, "results/vlm_example")
        
        # 示例3: 使用hybrid-auto-engine后端（下一代混合解决方案）
        print("\n" + "="*60)
        print("示例3: 使用hybrid-auto-engine后端解析")
        print("="*60)
        
        results3 = parser.parse_files(
            files=files_to_parse if files_to_parse else [],
            backend="hybrid-auto-engine",
            lang_list=["zh"],  # 不设置语言列表，让服务端自动检测
            parse_method="auto",  # 强制使用OCR方法
            formula_enable=True,
            table_enable=True,
            return_md=True,
            return_middle_json=True,  # 获取中间JSON
            return_model_output=True   # 获取模型输出
        )
        
        parser.save_results(results3, "results/hybrid_example")
        
    except Exception as e:
        print(f"❌ 解析过程中发生错误: {e}")
        import traceback
        traceback.print_exc()