#!/usr/bin/env python3
"""
数据库字段添加脚本：为现有表添加tenant_id字段
Phase 1: Data Model Completion - 数据库字段添加
"""

import os
import sys
import psycopg2
from psycopg2.extensions import ISOLATION_LEVEL_AUTOCOMMIT

# 添加项目根目录到Python路径
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from api.settings import DATABASE


def get_db_connection():
    """获取数据库连接"""
    try:
        conn = psycopg2.connect(
            host=DATABASE.get('host', 'localhost'),
            port=DATABASE.get('port', 5432),
            database=DATABASE.get('name'),
            user=DATABASE.get('user'),
            password=DATABASE.get('password')
        )
        conn.set_isolation_level(ISOLATION_LEVEL_AUTOCOMMIT)
        return conn
    except Exception as e:
        print(f"数据库连接失败: {e}")
        return None


def add_tenant_id_field(conn, table_name):
    """为指定表添加tenant_id字段"""
    try:
        cursor = conn.cursor()
        
        # 检查字段是否已存在
        check_query = """
        SELECT column_name 
        FROM information_schema.columns 
        WHERE table_name = %s AND column_name = 'tenant_id'
        """
        cursor.execute(check_query, (table_name,))
        
        if cursor.fetchone():
            print(f"表 {table_name} 已存在tenant_id字段，跳过")
            return True
            
        # 添加tenant_id字段
        add_column_query = f"""
        ALTER TABLE {table_name} 
        ADD COLUMN tenant_id VARCHAR(32) DEFAULT NULL
        """
        
        cursor.execute(add_column_query)
        print(f"成功为表 {table_name} 添加tenant_id字段")
        
        # 为tenant_id字段添加索引
        index_name = f"idx_{table_name}_tenant_id"
        add_index_query = f"""
        CREATE INDEX {index_name} ON {table_name}(tenant_id)
        """
        
        try:
            cursor.execute(add_index_query)
            print(f"成功为表 {table_name} 的tenant_id字段添加索引")
        except psycopg2.errors.DuplicateTable:
            print(f"表 {table_name} 的tenant_id索引已存在，跳过")
        
        cursor.close()
        return True
        
    except Exception as e:
        print(f"为表 {table_name} 添加tenant_id字段失败: {e}")
        return False


def add_foreign_key_constraint(conn, table_name):
    """添加外键约束"""
    try:
        cursor = conn.cursor()
        
        # 检查外键约束是否已存在
        check_fk_query = """
        SELECT constraint_name 
        FROM information_schema.table_constraints 
        WHERE table_name = %s AND constraint_type = 'FOREIGN KEY'
        AND constraint_name LIKE %s
        """
        cursor.execute(check_fk_query, (table_name, f'fk_{table_name}_tenant%'))
        
        if cursor.fetchone():
            print(f"表 {table_name} 已存在外键约束，跳过")
            cursor.close()
            return True
            
        # 添加外键约束
        fk_name = f"fk_{table_name}_tenant"
        add_fk_query = f"""
        ALTER TABLE {table_name} 
        ADD CONSTRAINT {fk_name} 
        FOREIGN KEY (tenant_id) REFERENCES tenant(id) ON DELETE CASCADE
        """
        
        cursor.execute(add_fk_query)
        print(f"成功为表 {table_name} 添加外键约束")
        
        cursor.close()
        return True
        
    except Exception as e:
        print(f"为表 {table_name} 添加外键约束失败: {e}")
        return False


def check_table_exists(conn, table_name):
    """检查表是否存在"""
    try:
        cursor = conn.cursor()
        cursor.execute("""
        SELECT EXISTS (
            SELECT FROM information_schema.tables 
            WHERE table_name = %s
        )
        """, (table_name,))
        exists = cursor.fetchone()[0]
        cursor.close()
        return exists
    except Exception as e:
        print(f"检查表 {table_name} 是否存在时出错: {e}")
        return False


def main():
    """主函数"""
    print("=== 开始添加tenant_id字段 ===")
    
    # 需要添加tenant_id字段的表
    tables = [
        'document',
        'conversation',
        'dialog',
        'knowledgebase',
        'llm',
        'user',
        'task'
    ]
    
    conn = get_db_connection()
    if not conn:
        print("无法连接数据库，操作终止")
        return False
        
    try:
        success_count = 0
        
        for table_name in tables:
            print(f"\n处理表: {table_name}")
            
            # 检查表是否存在
            if not check_table_exists(conn, table_name):
                print(f"表 {table_name} 不存在，跳过")
                continue
                
            # 添加tenant_id字段
            if add_tenant_id_field(conn, table_name):
                # 添加外键约束（tenant表必须已存在）
                if table_name != 'tenant':  # tenant表不需要外键约束
                    add_foreign_key_constraint(conn, table_name)
                success_count += 1
                
        print(f"\n=== 完成处理 {success_count} 个表 ===")
        return True
        
    except Exception as e:
        print(f"执行过程中发生错误: {e}")
        return False
    finally:
        if conn:
            conn.close()


if __name__ == "__main__":
    success = main()
    sys.exit(0 if success else 1)