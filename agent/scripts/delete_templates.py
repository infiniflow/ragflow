import os
import sys
import logging
import tkinter as tk
from tkinter import filedialog, messagebox
import pymysql

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

def show_template_selection(templates):
    """显示模板选择对话框"""
    if not templates:
        messagebox.showinfo("提示", "数据库中没有可删除的模板")
        return []
    
    # 创建窗口
    root = tk.Tk()
    root.title("选择要删除的模板")
    root.geometry("500x400")
    
    # 创建框架
    frame = tk.Frame(root)
    frame.pack(fill=tk.BOTH, expand=True, padx=10, pady=10)
    
    # 标题标签
    title_label = tk.Label(frame, text="选择要删除的模板 (可多选)", font=("Arial", 12))
    title_label.pack(pady=10)
    
    # 创建列表框和滚动条
    scrollbar = tk.Scrollbar(frame)
    scrollbar.pack(side=tk.RIGHT, fill=tk.Y)
    
    listbox = tk.Listbox(frame, selectmode=tk.MULTIPLE, width=60, height=15)
    listbox.pack(fill=tk.BOTH, expand=True)
    
    # 配置滚动条
    listbox.config(yscrollcommand=scrollbar.set)
    scrollbar.config(command=listbox.yview)
    
    # 填充列表
    template_map = {}
    for template_id, title in templates:
        display_text = f"{title} (ID: {template_id})"
        template_map[display_text] = template_id
        listbox.insert(tk.END, display_text)
    
    # 保存用户选择的模板ID
    selected_ids = []
    
    def on_confirm():
        selected_indices = listbox.curselection()
        for i in selected_indices:
            item = listbox.get(i)
            selected_ids.append(template_map[item])
        root.destroy()
    
    # 确认和取消按钮
    btn_frame = tk.Frame(frame)
    btn_frame.pack(pady=10)
    
    confirm_btn = tk.Button(btn_frame, text="确认删除", command=on_confirm, bg="#ff5555", fg="white")
    confirm_btn.pack(side=tk.LEFT, padx=10)
    
    cancel_btn = tk.Button(btn_frame, text="取消", command=root.destroy)
    cancel_btn.pack(side=tk.LEFT)
    
    # 运行窗口
    root.mainloop()
    
    return selected_ids

def delete_templates(conn, template_ids):
    """从数据库中删除选定的模板"""
    if not template_ids:
        logger.info("没有选择要删除的模板")
        return 0
    
    cursor = conn.cursor()
    deleted_count = 0
    
    try:
        for template_id in template_ids:
            cursor.execute("DELETE FROM canvas_template WHERE id = %s", (template_id,))
            deleted_count += cursor.rowcount
        
        conn.commit()
        logger.info(f"成功删除 {deleted_count} 个模板")
        return deleted_count
    except Exception as e:
        conn.rollback()
        logger.error(f"删除模板失败: {e}")
        return 0
    finally:
        cursor.close()

def main():
    """主函数"""
    # 连接数据库
    conn = setup_database()
    if not conn:
        return
    
    try:
        # 获取可用模板
        templates = get_available_templates(conn)
        
        # 显示模板选择对话框
        selected_template_ids = show_template_selection(templates)
        
        # 删除选定的模板
        deleted_count = delete_templates(conn, selected_template_ids)
        
        # 显示结果
        if deleted_count > 0:
            messagebox.showinfo("成功", f"成功删除 {deleted_count} 个模板")
        
    finally:
        conn.close()

if __name__ == "__main__":
    main()