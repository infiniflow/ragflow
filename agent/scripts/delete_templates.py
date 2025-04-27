import os
import sys
import logging
import pymysql
import glob
import shutil

# 配置日志
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)

# 基础目录和模板目录
BASE_DIR = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
TEMPLATES_DIR = os.path.join(BASE_DIR, "agent", "templates")

# 数据库配置
DB_CONFIG = {
    'host': 'localhost',
    'port': 5455,
    'user': 'root',
    'password': 'infini_rag_flow',
    'database': 'rag_flow'
}

def setup_database():
    """连接到MySQL数据库"""
    try:
        conn = pymysql.connect(**DB_CONFIG)
        logger.info("数据库连接成功")
        return conn
    except Exception as e:
        logger.error(f"数据库连接失败: {e}")
        return None

def get_available_templates(conn):
    """获取数据库中所有可用的模板"""
    cursor = conn.cursor()
    try:
        cursor.execute("SELECT id, title FROM canvas_template")
        templates = cursor.fetchall()
        return templates
    except Exception as e:
        logger.error(f"获取模板列表失败: {e}")
        return []
    finally:
        cursor.close()

def show_template_selection_cli(templates):
    """命令行版本的模板选择"""
    if not templates:
        print("数据库中没有可删除的模板")
        return []
    
    print("\n可用模板列表:")
    print("-------------------")
    for i, (template_id, title) in enumerate(templates, 1):
        print(f"{i}. {title} (ID: {template_id})")
    print("-------------------")
    
    selected_ids = []
    try:
        while True:
            selection = input("\n请输入要删除的模板编号(1-{}), 多个编号用逗号分隔, 输入'q'完成选择: ".format(len(templates)))
            
            if selection.lower() == 'q':
                break
                
            # 处理用户输入的编号
            try:
                indices = [int(idx.strip()) for idx in selection.split(',') if idx.strip()]
                for idx in indices:
                    if 1 <= idx <= len(templates):
                        template_id = templates[idx-1][0]
                        title = templates[idx-1][1]
                        if template_id not in selected_ids:
                            selected_ids.append(template_id)
                            print(f"已选择: {title} (ID: {template_id})")
                    else:
                        print(f"无效的编号: {idx}, 请输入1-{len(templates)}之间的数字")
            except ValueError:
                print("请输入有效的数字编号，多个编号用逗号分隔")
    except KeyboardInterrupt:
        print("\n选择已取消")
        return []
    
    if selected_ids:
        print(f"\n已选择 {len(selected_ids)} 个模板")
        confirm = input("确认删除这些模板? (y/n): ")
        if confirm.lower() != 'y':
            print("操作已取消")
            return []
    
    return selected_ids

def find_template_file_by_title(title):
    """根据模板标题查找可能的模板文件"""
    # 转换标题为可能的文件名模式
    # 例如："Text To SQL" -> "text_to_sql"
    possible_name = title.lower().replace(" ", "_")
    
    # 搜索可能的模板文件
    template_files = []
    patterns = [
        f"*{possible_name}*.py",
        f"*{possible_name.replace('_', '')}*.py",
        f"*{title.lower().replace(' ', '')}*.py"
    ]
    
    for pattern in patterns:
        search_path = os.path.join(TEMPLATES_DIR, pattern)
        found_files = glob.glob(search_path)
        template_files.extend(found_files)
    
    # 移除重复项
    return list(set(template_files))

def delete_template_files(template_title):
    """删除与模板标题匹配的模板文件"""
    template_files = find_template_file_by_title(template_title)
    deleted_files = []
    
    if template_files:
        print(f"找到与模板 '{template_title}' 可能关联的文件:")
        for i, file_path in enumerate(template_files, 1):
            print(f"  {i}. {os.path.basename(file_path)}")
        
        confirm = input("是否删除这些文件? (y/n): ")
        if confirm.lower() == 'y':
            for file_path in template_files:
                try:
                    # 创建备份
                    backup_path = file_path + '.bak'
                    shutil.copy2(file_path, backup_path)
                    print(f"已创建备份: {os.path.basename(backup_path)}")
                    
                    # 删除文件
                    os.remove(file_path)
                    print(f"已删除文件: {os.path.basename(file_path)}")
                    deleted_files.append(file_path)
                except Exception as e:
                    print(f"删除文件 {os.path.basename(file_path)} 时出错: {e}")
    else:
        print(f"未找到与模板 '{template_title}' 关联的文件")
    
    return deleted_files

def delete_templates(conn, template_ids, delete_files=True):
    """从数据库中删除选定的模板，并可选地删除关联的文件"""
    if not template_ids:
        logger.info("没有选择要删除的模板")
        return 0
    
    cursor = conn.cursor()
    deleted_count = 0
    deleted_files = []
    
    try:
        for template_id in template_ids:
            # 获取模板名称以便日志记录和查找文件
            cursor.execute("SELECT title FROM canvas_template WHERE id = %s", (template_id,))
            template_name = cursor.fetchone()
            
            if not template_name:
                print(f"警告: ID为{template_id}的模板在数据库中不存在")
                continue
                
            template_title = template_name[0]
            
            # 删除对应的模板文件
            if delete_files:
                files = delete_template_files(template_title)
                deleted_files.extend(files)
            
            # 执行数据库删除
            cursor.execute("DELETE FROM canvas_template WHERE id = %s", (template_id,))
            if cursor.rowcount > 0:
                deleted_count += cursor.rowcount
                print(f"已从数据库删除模板: {template_title} (ID: {template_id})")
        
        conn.commit()
        
        # 报告删除结果
        if deleted_count > 0:
            logger.info(f"成功从数据库删除 {deleted_count} 个模板")
        
        if deleted_files:
            logger.info(f"成功删除 {len(deleted_files)} 个模板文件")
            
        return deleted_count
    except Exception as e:
        conn.rollback()
        logger.error(f"删除模板失败: {e}")
        print(f"错误: 删除模板失败 - {e}")
        return 0
    finally:
        cursor.close()

def main():
    """主函数"""
    print("===== 模板删除工具 =====")
    
    # 连接数据库
    print("正在连接数据库...")
    conn = setup_database()
    if not conn:
        print("无法连接到数据库，程序退出")
        return
    
    try:
        # 获取可用模板
        print("正在获取可用模板...")
        templates = get_available_templates(conn)
        
        if not templates:
            print("数据库中没有可用的模板")
            return
            
        # 显示模板选择界面(命令行版本)
        selected_template_ids = show_template_selection_cli(templates)
        
        # 询问是否同时删除文件
        delete_files = True
        if selected_template_ids:
            file_choice = input("是否同时删除模板文件? (y/n, 默认y): ")
            delete_files = file_choice.lower() != 'n'
        
        # 删除选定的模板
        deleted_count = delete_templates(conn, selected_template_ids, delete_files)
        
        # 显示结果
        if deleted_count > 0:
            print(f"\n成功完成! 已从数据库删除 {deleted_count} 个模板")
        else:
            print("\n未删除任何模板")
        
    finally:
        conn.close()
        print("数据库连接已关闭")

if __name__ == "__main__":
    main()

# Execute the script from the specified directory
os.system("cd /home/user/ragflow/ragflow/agent/scripts && python delete_templates.py")