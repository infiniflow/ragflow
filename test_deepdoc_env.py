"""
DeepDoc 虚拟环境测试脚本

使用方法:
    1. 激活虚拟环境: .venv\Scripts\activate (Windows) 或 source .venv/bin/activate (Linux/Mac)
    2. 运行: python test_deepdoc_env.py
"""

import sys
import types

# 创建 datrie mock（因为 datrie 在 Windows 上编译困难）
datrie_mock = types.ModuleType('datrie')
class Trie:
    def __init__(self, *args, **kwargs):
        self._data = {}
    def __setitem__(self, key, value):
        self._data[key] = value
    def __getitem__(self, key):
        return self._data[key]
    def __contains__(self, key):
        return key in self._data
    def keys(self, prefix=''):
        return [k for k in self._data if k.startswith(prefix)]
    def has_keys_with_prefix(self, prefix):
        return any(k.startswith(prefix) for k in self._data)
    def longest_prefix(self, key):
        for i in range(len(key), 0, -1):
            if key[:i] in self._data:
                return key[:i]
        return ''
    def longest_prefix_item(self, key):
        prefix = self.longest_prefix(key)
        return (prefix, self._data.get(prefix, None)) if prefix else (None, None)
    @classmethod
    def load(cls, filename):
        return cls()
    def save(self, filename):
        pass

datrie_mock.Trie = Trie
sys.modules['datrie'] = datrie_mock

# 确保项目根目录在路径中
sys.path.insert(0, '.')

def test_imports():
    """测试 deepdoc 核心模块导入"""
    results = []

    modules = [
        ('deepdoc.vision', 'OCR'),
        ('deepdoc.vision.ocr', 'OCR'),
        ('deepdoc.vision.layout_recognizer', 'LayoutRecognizer'),
        ('deepdoc.vision.table_structure_recognizer', 'TableStructureRecognizer'),
        ('deepdoc.vision.recognizer', 'Recognizer'),
        ('deepdoc.parser.pdf_parser', 'RAGFlowPdfParser'),
        ('deepdoc.parser.docx_parser', 'RAGFlowDocxParser'),
    ]

    for module_name, class_name in modules:
        try:
            module = __import__(module_name, fromlist=[class_name])
            getattr(module, class_name)
            results.append((module_name, class_name, True, None))
        except Exception as e:
            results.append((module_name, class_name, False, str(e)))

    return results

def main():
    print("=" * 60)
    print("DeepDoc 虚拟环境测试")
    print("=" * 60)
    print()

    results = test_imports()

    success_count = 0
    fail_count = 0

    for module_name, class_name, success, error in results:
        status = "OK" if success else "FAIL"
        if success:
            success_count += 1
            print(f"[{status}] {module_name}.{class_name}")
        else:
            fail_count += 1
            print(f"[{status}] {module_name}.{class_name}: {error}")

    print()
    print("-" * 60)
    print(f"测试结果: {success_count} 成功, {fail_count} 失败")
    print("-" * 60)

    if fail_count == 0:
        print("所有模块导入成功！虚拟环境配置完成。")
    else:
        print("部分模块导入失败，请检查依赖安装。")

    return fail_count == 0

if __name__ == "__main__":
    success = main()
    sys.exit(0 if success else 1)
