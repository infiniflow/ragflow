#!/usr/bin/env python3
"""
调试 init_llm_factory 的问题
"""
import os
import sys
import json
import hashlib

# 设置项目根目录
project_root = os.path.dirname(os.path.abspath(__file__))
os.chdir(project_root)

# 添加项目路径
sys.path.insert(0, project_root)

print("=" * 60)
print("RAGFlow init_llm_factory 调试工具")
print("=" * 60)

# 1. 检查配置文件是否存在
conf_path = os.path.join(project_root, "conf", "llm_factories.json")
print(f"\n1. 检查配置文件:")
print(f"   路径: {conf_path}")
print(f"   存在: {os.path.exists(conf_path)}")

if os.path.exists(conf_path):
    print(f"   大小: {os.path.getsize(conf_path)} bytes")
    with open(conf_path, "r", encoding="utf-8") as f:
        data = json.load(f)
        factory_llm_infos = data.get("factory_llm_infos", [])
        print(f"   工厂数量: {len(factory_llm_infos)}")
        
        # 计算哈希
        content = json.dumps(factory_llm_infos, sort_keys=True, ensure_ascii=False)
        current_hash = hashlib.md5(content.encode()).hexdigest()
        print(f"   当前哈希: {current_hash}")

print("\n2. 检查环境变量:")
print(f"   PYTHONPATH: {os.environ.get('PYTHONPATH', '未设置')}")
print(f"   DB_TYPE: {os.environ.get('DB_TYPE', '未设置(默认mysql)')}")
print(f"   DOC_ENGINE: {os.environ.get('DOC_ENGINE', '未设置(默认elasticsearch)')}")

print("\n3. 尝试初始化数据库连接...")
try:
    from common import settings
    settings.init_settings()
    print("   设置初始化成功!")
    print(f"   FACTORY_LLM_INFOS 数量: {len(settings.FACTORY_LLM_INFOS) if settings.FACTORY_LLM_INFOS else 0}")
except Exception as e:
    print(f"   错误: {e}")
    import traceback
    traceback.print_exc()

print("\n4. 检查数据库中的哈希值...")
try:
    from api.db.services.system_settings_service import SystemSettingsService
    
    rows = list(SystemSettingsService.get_by_name("__llm_factory_hash__"))
    if rows:
        stored_hash = rows[0].value if hasattr(rows[0], "value") else None
        print(f"   数据库中存储的哈希: {stored_hash}")
    else:
        print("   数据库中没有找到哈希值 (首次运行或已被删除)")
except Exception as e:
    print(f"   错误: {e}")
    import traceback
    traceback.print_exc()

print("\n5. 对比哈希值...")
try:
    from common import settings
    if settings.FACTORY_LLM_INFOS:
        content = json.dumps(settings.FACTORY_LLM_INFOS, sort_keys=True, ensure_ascii=False)
        current_hash = hashlib.md5(content.encode()).hexdigest()
        
        rows = list(SystemSettingsService.get_by_name("__llm_factory_hash__"))
        if rows:
            stored_hash = rows[0].value if hasattr(rows[0], "value") else None
            
            if stored_hash == current_hash:
                print(f"   ✓ 哈希匹配! 应该跳过重建")
                print(f"   当前: {current_hash}")
                print(f"   存储: {stored_hash}")
            else:
                print(f"   ✗ 哈希不匹配! 将触发重建")
                print(f"   当前: {current_hash}")
                print(f"   存储: {stored_hash}")
        else:
            print(f"   没有存储的哈希值，将触发首次重建")
            print(f"   当前: {current_hash}")
except Exception as e:
    print(f"   错误: {e}")

print("\n" + "=" * 60)
print("调试完成")
print("=" * 60)
