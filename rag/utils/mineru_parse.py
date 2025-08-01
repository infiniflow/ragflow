# Copyright (c) 2024 Huawei Technologies Co.,Ltd. All rights reserved.
#
# AgentKernel is licensed under Mulan PSL v2.
# You can use this software according to the terms and conditions of the Mulan
# PSL v2.
# You may obtain a copy of Mulan PSL v2 at:
#         http://license.coscl.org.cn/MulanPSL2
# THIS SOFTWARE IS PROVIDED ON AN "AS IS" BASIS, WITHOUT WARRANTIES OF ANY
# KIND, EITHER EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO
# NON-INFRINGEMENT, MERCHANTABILITY OR FIT FOR A PARTICULAR PURPOSE.
# See the Mulan PSL v2 for more details.
import os
from typing import Union
from io import BytesIO
import logging
from deepdoc.parser import PdfParser


class MinerUPdf(PdfParser):
    def __init__(self):
        super().__init__()

    def __call__(self, filename, binary=None, from_page=0,
                 to_page=100000, zoomin=3, callback=None, separate_tables_figures=False):
        return self.mineru_parse(filename)

    def sanitize_input(self, fnm: Union[str, bytes]) -> BytesIO:
        """统一处理输入，确保返回安全的BytesIO对象"""
        try:
            if isinstance(fnm, str):
                # 处理文件路径：移除空字节并标准化路径
                sanitized_path = fnm.replace('\x00', '')
                sanitized_path = os.path.normpath(sanitized_path)
                with open(sanitized_path, 'rb') as f:
                    return BytesIO(f.read())
            elif isinstance(fnm, bytes):
                return BytesIO(fnm)
            elif hasattr(fnm, 'read'):  # 支持文件类对象
                return BytesIO(fnm.read())
            else:
                raise ValueError("Unsupported input type")
        except Exception as e:
            logging.error(f"Input sanitization failed: {str(e)}")
            raise RuntimeError("Failed to process input file") from e


    def mineru_parse(self, filename):
        try:
            # 统一输入处理
            pdf_stream = self.sanitize_input(filename)
            mineru_url = os.getenv('MINERU_URL')
            if mineru_url:
                from rag.utils.pdf_to_txts import convert_pdf_to_txts
                txts = convert_pdf_to_txts(pdf_stream)
                return txts, []
            else:
                raise RuntimeError(f"未检测到MinerU服务地址，请在环境变量中进行配置。")
        except Exception as e:
            logging.critical(f"PDF processing failed: {str(e)}")
            raise RuntimeError(f"PDF parsing error: {str(e)}") from e