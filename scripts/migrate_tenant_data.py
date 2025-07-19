#!/usr/bin/env python3
"""
数据迁移脚本：为现有Document和Conversation数据添加tenant_id字段
Phase 1: Data Model Completion - 数据迁移脚本
"""

import os
import sys
from peewee import *

# 添加项目根目录到Python路径
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from api.db.db_models import DB, Document, Conversation, Tenant


def get_default_tenant():
    """获取默认租户ID"""
    try:
        tenant = Tenant.select().limit(1).first()
        if tenant:
            return tenant.id
        else:
            # 如果没有租户，创建一个默认租户
            from api.utils import get_uuid
            tenant_id = get_uuid()
            Tenant.create(
                id=tenant_id,
                name="Default Tenant",
                status="1"
            )
            return tenant_id
    except Exception as e:
        print(f"获取默认租户失败: {e}")
        return None


def migrate_documents(tenant_id):
    """迁移Document数据"""
    print("开始迁移Document数据...")
    try:
        # 查找所有没有tenant_id的Document
        documents = Document.select().where(Document.tenant_id.is_null())
        count = documents.count()
        
        if count == 0:
            print("所有Document已有tenant_id，无需迁移")
            return True
            
        print(f"需要迁移的Document数量: {count}")
        
        # 批量更新
        query = Document.update(tenant_id=tenant_id).where(Document.tenant_id.is_null())
        updated = query.execute()
        
        print(f"成功更新 {updated} 个Document")
        return True
        
    except Exception as e:
        print(f"Document迁移失败: {e}")
        return False


def migrate_conversations(tenant_id):
    """迁移Conversation数据"""
    print("开始迁移Conversation数据...")
    try:
        # 查找所有没有tenant_id的Conversation
        conversations = Conversation.select().where(Conversation.tenant_id.is_null())
        count = conversations.count()
        
        if count == 0:
            print("所有Conversation已有tenant_id，无需迁移")
            return True
            
        print(f"需要迁移的Conversation数量: {count}")
        
        # 批量更新
        query = Conversation.update(tenant_id=tenant_id).where(Conversation.tenant_id.is_null())
        updated = query.execute()
        
        print(f"成功更新 {updated} 个Conversation")
        return True
        
    except Exception as e:
        print(f"Conversation迁移失败: {e}")
        return False


def verify_migration():
    """验证迁移结果"""
    print("开始验证迁移结果...")
    try:
        # 检查是否有未设置tenant_id的记录
        doc_null_count = Document.select().where(Document.tenant_id.is_null()).count()
        conv_null_count = Conversation.select().where(Conversation.tenant_id.is_null()).count()
        
        if doc_null_count > 0 or conv_null_count > 0:
            print(f"验证失败: Document未设置tenant_id数量: {doc_null_count}")
            print(f"验证失败: Conversation未设置tenant_id数量: {conv_null_count}")
            return False
            
        # 检查数据一致性
        doc_total = Document.select().count()
        conv_total = Conversation.select().count()
        
        print(f"验证成功: 总Document数量: {doc_total}")
        print(f"验证成功: 总Conversation数量: {conv_total}")
        
        return True
        
    except Exception as e:
        print(f"验证失败: {e}")
        return False


def main():
    """主函数"""
    print("=== RAGFlow多租户数据迁移开始 ===")
    
    try:
        # 获取数据库连接
        DB.connect()
        
        # 获取默认租户ID
        tenant_id = get_default_tenant()
        if not tenant_id:
            print("无法获取默认租户ID，迁移终止")
            return False
            
        print(f"使用默认租户ID: {tenant_id}")
        
        # 执行迁移
        doc_success = migrate_documents(tenant_id)
        conv_success = migrate_conversations(tenant_id)
        
        if doc_success and conv_success:
            # 验证迁移结果
            if verify_migration():
                print("=== 数据迁移成功完成 ===")
                return True
            else:
                print("=== 数据迁移验证失败 ===")
                return False
        else:
            print("=== 数据迁移执行失败 ===")
            return False
            
    except Exception as e:
        print(f"迁移过程中发生错误: {e}")
        return False
    finally:
        if not DB.is_closed():
            DB.close()


if __name__ == "__main__":
    success = main()
    sys.exit(0 if success else 1)