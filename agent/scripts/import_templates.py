import os
import sys
import json
import logging
import argparse
from typing import Dict, List, Any

# 配置日志
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)

# 基础目录
BASE_DIR = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
# 模板目录路径
TEMPLATES_DIR = os.path.join(BASE_DIR, "agent", "templates")

# 正确添加路径以导入模块
sys.path.append(BASE_DIR)
sys.path.append(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
sys.path.append(os.path.dirname(os.path.abspath(__file__)))

def read_json_file(file_path: str) -> Dict[str, Any]:
    """读取JSON文件并返回解析后的内容"""
    try:
        with open(file_path, 'r', encoding='utf-8') as f:
            return json.load(f)
    except Exception as e:
        logger.error(f"读取文件 {file_path} 失败: {str(e)}")
        return {}

def import_templates_using_update_module(template_files: List[str]) -> None:
    """使用update_template模块导入模板"""
    # 更可靠的导入方式
    update_template_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "update_template.py")
    
    if not os.path.exists(update_template_path):
        logger.error(f"找不到update_template.py文件，路径: {update_template_path}")
        return
    
    # 直接导入同目录下的update_template模块
    try:
        from agent.scripts.update_template import import_template
        logger.info("成功导入update_template模块")
    except ImportError as e:
        logger.error(f"无法导入update_template模块: {e}")
        # 尝试替代方案 - 直接执行update_template.py
        try:
            import importlib.util
            spec = importlib.util.spec_from_file_location("update_template", update_template_path)
            update_template = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(update_template)
            import_template = update_template.import_template
            logger.info("使用动态导入成功加载update_template模块")
        except Exception as e:
            logger.error(f"动态导入update_template失败: {e}")
            return

    # 导入模板
    success_count = 0
    fail_count = 0
    
    for file_path in template_files:
        logger.info(f"正在导入模板: {file_path}")
        try:
            result = import_template(file_path)
            if result:
                logger.info(f"成功导入模板: {os.path.basename(file_path)}")
                success_count += 1
            else:
                logger.error(f"导入模板失败: {os.path.basename(file_path)}")
                fail_count += 1
        except Exception as e:
            logger.error(f"导入模板时出错: {str(e)}")
            fail_count += 1
    
    logger.info(f"模板导入完成: 成功 {success_count} 个, 失败 {fail_count} 个")

def main():
    """主函数"""
    # 创建参数解析器
    parser = argparse.ArgumentParser(description='导入模板JSON文件到系统')
    parser.add_argument('template_files', nargs='+', help='要导入的JSON模板文件路径')
    
    # 解析命令行参数
    args = parser.parse_args()
    template_files = args.template_files
    
    if not template_files:
        logger.warning("没有指定任何模板文件")
        return
    
    # 检查文件是否存在
    valid_files = []
    for file_path in template_files:
        if not os.path.exists(file_path):
            logger.warning(f"文件不存在: {file_path}")
            continue
        if not file_path.lower().endswith('.json'):
            logger.warning(f"文件不是JSON格式: {file_path}")
            continue
        valid_files.append(file_path)
    
    if not valid_files:
        logger.warning("没有找到有效的JSON模板文件")
        return
    
    # 使用update_template模块导入模板
    import_templates_using_update_module(valid_files)

if __name__ == "__main__":
    main()
