import json
from typing import List, Tuple
from rag.app.naive import chunk
from rag.prompts.prompts import toc_transformer, table_of_contents_index
from rag.nlp import num_tokens_from_string
from rag.prompts.prompts import detect_table_of_contents
from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from api.db.services.tenant_llm_service import TenantLLMService
from api.db.services.user_service import TenantService

if __name__ == "__main__":
    import sys
    
    from api import settings
    if settings.FACTORY_LLM_INFOS is None:
        print("Fixing FACTORY_LLM_INFOS initialization...")
        settings.init_settings()  # 重新初始化设置

    def dummy(prog=None, msg=""):
        pass
    tenant_id = "10b8ea16937911f09ae613abffb949cc"  # 从数据库查询到的用户ID
    
    results, tables, figures = chunk(sys.argv[1], from_page=0, to_page=10, callback=dummy, tenant_id=tenant_id)
    sections, section_images, page_1024, tc_arr = [], [], [""], [0]
    
    # 修复：results是元组列表，不是字典列表
    for text, image in results or []:
        tc = num_tokens_from_string(text)
        page_1024[-1] += "\n" + text
        tc_arr[-1] += tc
        if tc_arr[-1] > 1024:
            page_1024.append("")
            tc_arr.append(0)
    import sys
    from api import settings
    def dummy(prog=None, msg=""):
        pass

    def process_toc_full(pdf_path, tenant_id):
        if settings.FACTORY_LLM_INFOS is None:
            print("Fixing FACTORY_LLM_INFOS initialization...")
            settings.init_settings()
        results, tables, figures = chunk(pdf_path, from_page=0, to_page=10, callback=dummy, tenant_id=tenant_id)
        sections, section_images, page_1024, tc_arr = [], [], [""], [0]
        for text, image in results or []:
            tc = num_tokens_from_string(text)
            page_1024[-1] += "\n" + text
            tc_arr[-1] += tc
            if tc_arr[-1] > 1024:
                page_1024.append("")
                tc_arr.append(0)
            sections.append((text, ""))
            section_images.append(image)
        chat_mdl = LLMBundle(tenant_id, LLMType.CHAT, llm_name="deepseek-ai/DeepSeek-R1-Distill-Qwen-7B", lang="Chinese")
        toc_secs = detect_table_of_contents(page_1024, chat_mdl)
        with open("toc_detection_result.txt", "w", encoding="utf-8") as f:
            f.write("=== TOC Detection Results ===\n")
            f.write(f"Found {len(toc_secs)} TOC sections\n\n")
            for i, sec in enumerate(toc_secs):
                f.write(f"--- Section {i+1} ---\n")
                f.write(sec)
                f.write("\n\n")
        print(f"✅ TOC detection results saved to toc_detection_result.txt ({len(toc_secs)} sections)")
        if toc_secs:
            toc_arr = toc_transformer(toc_secs, chat_mdl)
            with open("toc_transformer_result.txt", "w", encoding="utf-8") as f:
                f.write("=== TOC Transformer Results ===\n")
                f.write(json.dumps(toc_arr, ensure_ascii=False, indent=2))
            print(f"✅ TOC transformer results saved to toc_transformer_result.txt ({len(toc_arr)} items)")
            toc_arr = [it for it in toc_arr if it.get("structure")]
            print(f"📋 Filtered to {len(toc_arr)} items with structure")
            toc_arr = table_of_contents_index(toc_arr, [t for t,_ in sections], chat_mdl)
            with open("toc_index_result.txt", "w", encoding="utf-8") as f:
                f.write("=== TOC Index Results ===\n")
                f.write(json.dumps(toc_arr, ensure_ascii=False, indent=2))
            print(f"✅ TOC index results saved to toc_index_result.txt ({len(toc_arr)} items)")
            print("\n" + "="*50)
            print("FINAL TOC STRUCTURE:")
            print("="*50)
            print(json.dumps(toc_arr, ensure_ascii=False, indent=2), flush=True)
        else:
            print("❌ No TOC sections detected")

    def process_toc_from_file(tenant_id, sections_path, toc_transformer_path):
        if settings.FACTORY_LLM_INFOS is None:
            print("Fixing FACTORY_LLM_INFOS initialization...")
            settings.init_settings()
        # 读取sections
        with open(sections_path, "r", encoding="utf-8") as f:
            sections = [line.strip() for line in f if line.strip() and not line.startswith("===") and not line.startswith("---")]
        # 读取toc_transformer结果
        with open(toc_transformer_path, "r", encoding="utf-8") as f:
            toc_arr = json.loads(f.read().split("=== TOC Transformer Results ===\n")[-1])
        chat_mdl = LLMBundle(tenant_id, LLMType.CHAT, llm_name="deepseek-ai/DeepSeek-R1-Distill-Qwen-7B", lang="Chinese")
        toc_arr = [it for it in toc_arr if it.get("structure")]
        print(f"📋 Filtered to {len(toc_arr)} items with structure")
        toc_arr = table_of_contents_index(toc_arr, sections, chat_mdl)
        with open("toc_index_result.txt", "w", encoding="utf-8") as f:
            f.write("=== TOC Index Results ===\n")
            f.write(json.dumps(toc_arr, ensure_ascii=False, indent=2))
        print(f"✅ TOC index results saved to toc_index_result.txt ({len(toc_arr)} items)")
        print("\n" + "="*50)
        print("FINAL TOC STRUCTURE:")
        print("="*50)
        print(json.dumps(toc_arr, ensure_ascii=False, indent=2), flush=True)

    if __name__ == "__main__":
        # 示例：只执行第三步，前两步结果从本地文件读取
        tenant_id = "10b8ea16937911f09ae613abffb949cc"
        process_toc_from_file(tenant_id, "toc_detection_result.txt", "toc_transformer_result.txt")