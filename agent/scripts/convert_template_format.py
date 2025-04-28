#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
转换普通Agent JSON文件为标准模板格式
用法：python convert_template_format.py 输入文件.json [输出文件.json]
"""

import os
import sys
import json
import argparse
import logging
from typing import Dict, Any, List, Optional

# 配置日志
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)

def load_json_file(file_path: str) -> Dict[str, Any]:
    """
    加载JSON文件并返回解析后的内容
    """
    try:
        with open(file_path, 'r', encoding='utf-8') as f:
            return json.load(f)
    except Exception as e:
        logger.error(f"读取文件 {file_path} 失败: {str(e)}")
        sys.exit(1)

def save_json_file(data: Dict[str, Any], file_path: str) -> None:
    """
    保存JSON数据到文件
    """
    try:
        with open(file_path, 'w', encoding='utf-8') as f:
            json.dump(data, f, ensure_ascii=False, indent=2)
        logger.info(f"已成功保存到 {file_path}")
    except Exception as e:
        logger.error(f"保存文件 {file_path} 失败: {str(e)}")
        sys.exit(1)

def convert_to_standard_format(input_data: Dict[str, Any], template_id: int = 10) -> Dict[str, Any]:
    """
    将普通Agent JSON格式转换为标准模板格式
    """
    # 获取输入数据中的关键内容
    components = input_data.get("components", {})
    edges = input_data.get("edges", [])
    nodes = input_data.get("nodes", [])
    
    # 创建标准模板结构
    standard_template = {
        "id": template_id,
        "title": input_data.get("title", "转换的模板"),
        "description": input_data.get("description", "由普通Agent转换生成的标准模板"),
        "canvas_type": "chatbot",
        "dsl": {
            "answer": [],
            "components": components,
            "embed_id": "",
            "graph": {
                "edges": edges,
                "nodes": nodes
            },
            "history": [],
            "messages": [],
            "path": [],
            "reference": []
        },
        "avatar": input_data.get("avatar", "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI1MCIgaGVpZ2h0PSI1MCIgdmlld0JveD0iMCAwIDUwIDUwIiBmaWxsPSJub25lIj4KICA8cmVjdCB3aWR0aD0iNTAiIGhlaWdodD0iNTAiIGZpbGw9IiMyQjc0RkYiIHJ4PSI4Ii8+CiAgPHBhdGggZmlsbD0id2hpdGUiIGQ9Ik0xOC4zMzM5IDI1LjM4NTdDMjAuMzY0NiAyMi41MzgxIDIzLjYwNTYgMjAuNiAyNy43MjkgMjAuMkMyOC4yODUzIDIwLjE0MjkgMjguNDg1OSAyMC43NDI5IDI4LjA1NzUgMjEuMTQyOUMyNS4xNDM5IDIzLjg1NzEgMjEuNjQ1MiAyNC44ODU3IDIwLjMwNzUgMjcuNDI4NkMyMC4xNjM5IDI3LjY4NTcgMjAuMDc3NSAyNy45NzE0IDIwLjAxMzkgMjguMjY4NkMyMC4wNTIxIDI4LjI1NzEgMjAuMDkwMyAyOC4yNDU3IDIwLjEyODUgMjguMjIyOUMyMC45MDMgMjcuODExNCAyMS43NDMyIDI3LjY1NzEgMjIuNjI5MiAyNy43NzE0QzIzLjUzNjcgMjcuODg1NyAyNC4zNzY5IDI4LjM2ODYgMjUuMDQ3NiAyOS4wNTcxQzI1LjcxODIgMjkuNzM0MyAyNi4xOTIxIDMwLjU4ODYgMjYuMzAyOCAzMS40ODU3QzI2LjQxMzYgMzIuMzgyOSAyNi4yNTg0IDMzLjI5MTQgMjUuODQyOSAzNC4wNTcxQzI1LjQzOSAzNC44MTE0IDI0Ljc4MTIgMzUuNDM0MyAyMy45OTU3IDM1LjgzNDNDMjMuMjIxMSAzNi4yMzQzIDIyLjMxOTIgMzYuNCAyMS40MjMzIDM2LjI4NTdDMjAuNTI3NCAzNi4xNzE0IDE5LjY4NzIgMzUuNjg4NiAxOS4wMTY1IDM1QzE4LjM0NTkgMzQuMzIyOSAxNy44NzIgMzMuNDY4NiAxNy43NjEzIDMyLjU3MTRDMTcuNjUwNSAzMS42NzQzIDE3LjgwNTggMzAuNzY1NyAxOC4yMjEzIDMwQzE4LjYzNjggMjkuMjM0MyAxOS4yOTQ2IDI4LjYxMTQgMjAuMDgwMSAyOC4yMTE0QzIwLjA0MTkgMjguMjIyOSAyMC4wMDM3IDI4LjIzNDMgMTkuOTY1NSAyOC4yNDU3QzE4Ljg1MTIgMjguODU3MSAxOC4wNDQgMjkuOTQyOSAxNy42Mzk0IDMxLjE3MTRDMTcuMTk5NSAzMi40OTE0IDE3LjIzNzcgMzMuOTQyOSAxNy43NjEzIDM1LjIzNDNDMTguMjg0OCAzNi41MjU3IDE5LjI2MzMgMzcuNjExNCAxMC41NTQxIDM4LjE3MTRDMjEuODQ0OCAzOC43MiAyMy4yMjExIDM4LjcwODYgMjQuNDgyIDM4LjE3MTRDMjUuNzQyOSAzNy42MzQzIDI2LjgwNjUgMzYuNjQ1NyAyNy41MTY2IDM1LjQxMTRDMjguMTY1MiAzNC4yIDI4LjQzMTkgMzIuNzk0MyAyOC4yNzM4IDMxLjRDMjguMTE1NyAzMC4wMDU3IDI3LjU0NjYgMjguNjg1NyAyNi42MjQ5IDI3LjY1NzFDMjUuNjkxOCAyNi42MTcxIDI0LjQ1NTQgMjUuODk3MSAyMy4wNzg2IDI1LjYwQzIyLjM4ODUgMjUuNDQ1NyAyMS42ODY3IDI1LjM4NTcgMjAuOTg0OCwyNS40MzQzQzIzLjAyNzMgMjEuOTY1NyAyNy4wNDY1IDE3LjggMzMuNTEwOSAyMy44QzMzLjcwMTUgMjMuOTc3MSAzMy41NDYzIDI0LjI5NzEgMzMuMjY4IDI0LjE3NzFDMjkuODYwOCAyMi44NTcxIDI2Ljg0NDcgMjIuOTAyOSAyNC40MzM4IDI0LjEwODZDMjcuMTM0OCAyMi40OTcxIDMwLjMzMzMgMjEuOTg4NiAzMy40NDQzIDIyLjY0MjlDMzMuNzAxNSAyMi42ODU3IDMzLjgzMjggMjMuMDE3MSAzMy41OTMyIDIzLjJDMjkuNDA4MyAyNi41NzE0IDI1LjI5NyAyNy4yIDE4Ljc1MDEgMzMuNDExNEMxOC41OTM0IDMzLjU2NTcgMTguMzUzNiAzMy42MTE0IDE4LjE0MjMgMzMuNUMxNy45MzEgMzMuMzg4NiAxNy44MDExIDMzLjEzNzEgMTcuODM5MyAzMi44NjI5QzE3LjgzOTMgMjkuNjkxNCAxNy42Mjk2IDI3LjU4MjkgMTguMzMzOSAyNS4zODU3WiIvPgo8L3N2Zz4K")
    }
    
    return standard_template

def main():
    # 解析命令行参数
    parser = argparse.ArgumentParser(description='将普通Agent JSON文件转换为标准模板格式')
    parser.add_argument('input_file', help='输入的普通Agent JSON文件路径')
    parser.add_argument('output_file', nargs='?', help='输出的标准模板JSON文件路径 (默认为输入文件名_标准.json)')
    parser.add_argument('-i', '--id', type=int, default=10, help='模板ID (默认为10)')
    args = parser.parse_args()
    
    input_file = args.input_file
    if not os.path.exists(input_file):
        logger.error(f"输入文件不存在: {input_file}")
        sys.exit(1)
    
    # 如果未指定输出文件，则使用默认命名
    if args.output_file:
        output_file = args.output_file
    else:
        base_name = os.path.splitext(os.path.basename(input_file))[0]
        output_file = os.path.join(os.path.dirname(input_file), f"{base_name}_标准.json")
    
    # 加载原始JSON数据
    logger.info(f"读取输入文件: {input_file}")
    input_data = load_json_file(input_file)
    
    # 转换为标准格式
    logger.info("转换为标准模板格式...")
    standard_data = convert_to_standard_format(input_data, args.id)
    
    # 保存转换后的JSON数据
    logger.info(f"保存标准模板到: {output_file}")
    save_json_file(standard_data, output_file)
    
    logger.info("转换完成！")

if __name__ == "__main__":
    main()