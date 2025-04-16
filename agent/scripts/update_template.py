import json
import pymysql
import os
import sys
import shutil
import importlib.util
import inspect
from pathlib import Path
import subprocess

# 数据库连接信息
db_config = {
    'host': 'localhost',
    'port': 5455,
    'user': 'root',
    'password': 'infini_rag_flow',
    'database': 'rag_flow'
}

# 直接使用指定的模板目录
TEMPLATES_DIR = r"D:\OneDrive\3_Code\ragflow\agent\templates"
BASE_DIR = r"D:\OneDrive\3_Code\ragflow"

def load_module_from_path(file_path, module_name):
    """从文件路径加载Python模块"""
    spec = importlib.util.spec_from_file_location(module_name, file_path)
    if spec is None:
        return None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module

def find_template_files():
    """查找模板目录中的Python文件"""
    if not os.path.exists(TEMPLATES_DIR):
        print(f"模板目录不存在: {TEMPLATES_DIR}")
        return []
        
    template_files = []
    for root, _, files in os.walk(TEMPLATES_DIR):
        for file in files:
            if file.endswith('.py'):
                template_files.append(os.path.join(root, file))
    return template_files

def analyze_templates():
    """分析模板文件"""
    template_files = find_template_files()
    
    if not template_files:
        print("未找到模板文件")
        return
    
    print(f"\n===== 分析模板目录 =====")
    print(f"找到 {len(template_files)} 个Python模板文件")
    
    for file_path in template_files:
        print(f"\n模板文件: {os.path.basename(file_path)}")
        
        # 分析文件内容
        with open(file_path, 'r', encoding='utf-8') as f:
            content = f.readlines()
        
        # 找出类定义
        class_lines = [line.strip() for line in content if line.strip().startswith("class ")]
        if class_lines:
            print("  包含的类:")
            for line in class_lines:
                print(f"    {line}")
        
        # 分析导入语句
        import_lines = [line.strip() for line in content if line.strip().startswith("import ") or line.strip().startswith("from ")]
        if import_lines:
            print("  导入语句:")
            for line in import_lines:
                print(f"    {line}")
        
        # 尝试加载模块并分析类结构
        try:
            module_name = os.path.basename(file_path).replace('.py', '')
            module = load_module_from_path(file_path, module_name)
            
            if module:
                for name in dir(module):
                    item = getattr(module, name)
                    if isinstance(item, type) and not name.startswith('__'):
                        print(f"  类: {name}")
                        # 获取类的继承树
                        print(f"    继承关系: {' -> '.join([c.__name__ for c in item.__mro__ if c.__name__ != 'object'])}")
                        
                        # 获取类的方法
                        methods = [name for name, member in inspect.getmembers(item) 
                                if inspect.isfunction(member) and not name.startswith('__')]
                        if methods:
                            print(f"    方法列表:")
                            for method in methods:
                                print(f"      - {method}")
        except Exception as e:
            print(f"  分析模块时出错: {e}")

def create_template_backup(template_path):
    """创建模板文件的备份"""
    backup_path = template_path + '.backup'
    shutil.copy2(template_path, backup_path)
    print(f"已创建备份: {backup_path}")
    return backup_path

def modify_template():
    """修改现有模板"""
    template_files = find_template_files()
    
    if not template_files:
        print("未找到模板文件")
        return
    
    print(f"\n找到 {len(template_files)} 个模板文件:")
    for i, file_path in enumerate(template_files, 1):
        print(f"{i}. {os.path.basename(file_path)}")
    
    try:
        choice = int(input("\n请选择要修改的模板文件 (输入序号): "))
        if choice < 1 or choice > len(template_files):
            print("无效选择")
            return
        
        selected_template = template_files[choice-1]
        
        # 尝试加载模块并分析类结构
        module_name = os.path.basename(selected_template).replace('.py', '')
        template_classes = []
        
        try:
            module = load_module_from_path(selected_template, module_name)
            if module:
                for name in dir(module):
                    item = getattr(module, name)
                    if isinstance(item, type) and not name.startswith('__'):
                        template_classes.append((name, item))
        except Exception as e:
            print(f"分析模块时出错: {e}")
        
        print("\n可执行的操作:")
        print("1. 添加新方法到现有类")
        print("2. 创建继承现有模板的新子类")
        
        op_choice = int(input("请选择操作 (输入序号): "))
        
        if op_choice == 1:
            if not template_classes:
                print("在所选模板中未找到可修改的类")
                return
                
            print("\n可修改的类:")
            for i, (class_name, _) in enumerate(template_classes, 1):
                print(f"{i}. {class_name}")
                
            class_choice = int(input("请选择要修改的类 (输入序号): "))
            if class_choice < 1 or class_choice > len(template_classes):
                print("无效选择")
                return
                
            selected_class_name = template_classes[class_choice-1][0]
            method_name = input("新方法名称: ")
            
            print("请输入方法代码 (输入'EOF'单独一行结束):")
            method_code_lines = []
            while True:
                line = input()
                if line == "EOF":
                    break
                method_code_lines.append(line)
            
            method_code = "\n".join(method_code_lines)
            
            # 读取文件内容
            with open(selected_template, 'r', encoding='utf-8') as f:
                lines = f.readlines()
            
            modified_lines = lines.copy()
            class_found = False
            insert_pos = -1
            
            # 寻找类定义和插入位置
            for i, line in enumerate(lines):
                if f"class {selected_class_name}" in line:
                    class_found = True
                elif class_found and line.strip() == "" and i < len(lines) - 1 and not lines[i+1].startswith("    "):
                    # 找到类结束的位置
                    insert_pos = i
                    break
            
            if insert_pos > 0:
                # 准备插入新方法
                method_lines = [f"    def {method_name}(self, *args, **kwargs):\n"]
                for code_line in method_code.split("\n"):
                    method_lines.append(f"        {code_line}\n")
                method_lines.append("\n")
                
                # 插入新方法
                modified_lines = lines[:insert_pos] + method_lines + lines[insert_pos:]
                
                # 创建备份并写入新文件
                create_template_backup(selected_template)
                with open(selected_template, 'w', encoding='utf-8') as f:
                    f.writelines(modified_lines)
                
                print(f"方法 '{method_name}' 已添加到类 '{selected_class_name}'")
            else:
                print(f"无法找到类 '{selected_class_name}' 或适合插入的位置")
            
        elif op_choice == 2:
            if not template_classes:
                print("在所选模板中未找到可继承的类")
                return
                
            print("\n可继承的类:")
            for i, (class_name, _) in enumerate(template_classes, 1):
                print(f"{i}. {class_name}")
                
            class_choice = int(input("请选择要继承的父类 (输入序号): "))
            if class_choice < 1 or class_choice > len(template_classes):
                print("无效选择")
                return
                
            parent_class_name = template_classes[class_choice-1][0]
            new_class_name = input("新子类名称: ")
            new_file_name = input("新模板文件名 (无需.py后缀): ") + ".py"
            new_file_path = os.path.join(TEMPLATES_DIR, new_file_name)
            
            if os.path.exists(new_file_path):
                overwrite = input(f"文件 {new_file_name} 已存在，是否覆盖? (y/n): ")
                if overwrite.lower() != 'y':
                    print("操作已取消")
                    return
            
            # 创建新的子类模板文件
            with open(new_file_path, 'w', encoding='utf-8') as f:
                parent_module = os.path.basename(selected_template).replace('.py', '')
                f.write(f"from .{parent_module} import {parent_class_name}\n\n")
                f.write(f"class {new_class_name}({parent_class_name}):\n")
                f.write(f"    \"\"\"{new_class_name} extends {parent_class_name} with custom functionality.\"\"\"\n\n")
                f.write(f"    def __init__(self, *args, **kwargs):\n")
                f.write(f"        super().__init__(*args, **kwargs)\n")
                f.write(f"        # 在这里添加自定义初始化代码\n\n")
                f.write(f"    # 在这里添加新方法或重写父类方法\n")
            
            print(f"新的子类模板 '{new_class_name}' 已创建在 '{new_file_path}'")
            
        else:
            print("无效选择")
            
    except ValueError:
        print("输入无效，请输入数字")
    except Exception as e:
        print(f"发生错误: {e}")
        import traceback
        traceback.print_exc()

def create_new_template():
    """创建新的agent模板文件"""
    if not os.path.exists(TEMPLATES_DIR):
        print(f"模板目录不存在: {TEMPLATES_DIR}")
        try:
            os.makedirs(TEMPLATES_DIR)
            print(f"已创建模板目录: {TEMPLATES_DIR}")
        except Exception as e:
            print(f"创建目录失败: {e}")
            return
    
    template_name = input("请输入新模板名称 (CamelCase格式): ")
    file_name = f"{template_name.lower()}_template.py"
    file_path = os.path.join(TEMPLATES_DIR, file_name)
    
    if os.path.exists(file_path):
        overwrite = input(f"文件 {file_name} 已存在，是否覆盖? (y/n): ")
        if overwrite.lower() != 'y':
            print("操作已取消")
            return
    
    # 获取基类信息
    base_template = input("请输入要继承的基类名称 (留空表示不继承): ")
    if base_template:
        base_module = input("基类所在模块 (例如 base_template): ")
        import_line = f"from .{base_module} import {base_template}"
        class_def = f"class {template_name}Template({base_template}):"
    else:
        import_line = "from typing import Dict, List, Any, Optional"
        class_def = f"class {template_name}Template:"
    
    # 创建模板内容
    template_content = [
        "# -*- coding: utf-8 -*-",
        '"""',
        f"{template_name} Template - 自定义agent模板",
        "",
        "这个模板提供了..." # 用户可以在运行时填充
        '"""',
        "",
        import_line,
        "",
        class_def,
        f'    """{template_name}Template 提供了自定义的agent行为."""',
        "",
        "    def __init__(self, *args, **kwargs):",
        "        super().__init__(*args, **kwargs) if hasattr(self, '__class__') and '.' in self.__class__.__module__ else None",
        "        self.template_name = '{template_name}'",
        "        # 在这里添加自定义初始化代码",
        "",
        "    def process_input(self, input_data: Dict[str, Any]) -> Dict[str, Any]:",
        '        """处理输入数据"""',
        "        # 在这里添加自定义输入处理逻辑",
        "        return input_data",
        "",
        "    def generate_response(self, context: Dict[str, Any]) -> Dict[str, Any]:",
        '        """生成响应"""',
        "        # 在这里添加自定义响应生成逻辑",
        "        response = {'status': 'success', 'message': 'Template response'}",
        "        return response",
        "",
        "    # 添加其他自定义方法"
    ]
    
    with open(file_path, 'w', encoding='utf-8') as f:
        f.write("\n".join(template_content))
    
    print(f"新模板已创建: {file_path}")
    
    # 检查并创建单元测试文件
    create_test = input("是否创建对应的单元测试文件? (y/n): ")
    if create_test.lower() == 'y':
        test_dir = os.path.join(os.path.dirname(TEMPLATES_DIR), "tests")
        if not os.path.exists(test_dir):
            try:
                os.makedirs(test_dir)
            except Exception as e:
                print(f"创建测试目录失败: {e}")
                return
        
        test_file_name = f"test_{file_name}"
        test_file_path = os.path.join(test_dir, test_file_name)
        
        test_content = [
            "# -*- coding: utf-8 -*-",
            '"""',
            f"单元测试 - {template_name}Template",
            '"""',
            "",
            "import unittest",
            f"from ..templates.{file_name[:-3]} import {template_name}Template",
            "",
            f"class Test{template_name}Template(unittest.TestCase):",
            "    def setUp(self):",
            f"        self.template = {template_name}Template()",
            "",
            "    def test_initialization(self):",
            f'        self.assertEqual(self.template.template_name, "{template_name}")',
            "",
            "    def test_process_input(self):",
            "        input_data = {'test': 'data'}",
            "        result = self.template.process_input(input_data)",
            "        self.assertIsNotNone(result)",
            "",
            "    def test_generate_response(self):",
            "        context = {'test': 'context'}",
            "        response = self.template.generate_response(context)",
            "        self.assertIn('status', response)",
            "",
            "if __name__ == '__main__':",
            "    unittest.main()"
        ]
        
        with open(test_file_path, 'w', encoding='utf-8') as f:
            f.write("\n".join(test_content))
        
        print(f"单元测试文件已创建: {test_file_path}")

def run_analyzer_script():
    """运行PowerShell分析脚本"""
    analyzer_path = os.path.join(BASE_DIR, "template_analyzer.ps1")
    
    # 检查分析脚本是否存在，不存在则创建
    if not os.path.exists(analyzer_path):
        create_analyzer_script(analyzer_path)
    
    try:
        print("运行模板分析脚本...")
        result = subprocess.run(
            ["powershell", "-ExecutionPolicy", "Bypass", "-File", analyzer_path],
            capture_output=True,
            text=True
        )
        print(result.stdout)
        if result.stderr:
            print("错误信息:")
            print(result.stderr)
    except Exception as e:
        print(f"运行脚本时出错: {e}")

def create_analyzer_script(path):
    """创建模板分析PowerShell脚本"""
    script_content = [
        "$templatesPath = \"D:\\OneDrive\\3_Code\\ragflow\\agent\\templates\"",
        "",
        "# 检查目录是否存在",
        "if (-Not (Test-Path $templatesPath)) {",
        "    Write-Host \"模板目录不存在: $templatesPath\" -ForegroundColor Red",
        "    exit",
        "}",
        "",
        "# 分析模板文件",
        "Write-Host \"===== 分析模板目录 =====\" -ForegroundColor Cyan",
        "$templateFiles = Get-ChildItem -Path $templatesPath -Recurse -File | Where-Object { $_.Extension -eq \".py\" }",
        "",
        "Write-Host \"找到 $($templateFiles.Count) 个Python模板文件\" -ForegroundColor Green",
        "",
        "# 显示模板文件列表",
        "Write-Host \"`n模板文件列表:\" -ForegroundColor Yellow",
        "foreach ($file in $templateFiles) {",
        "    Write-Host \"  $($file.FullName)\" -ForegroundColor White",
        "    ",
        "    # 显示文件内容概要",
        "    $content = Get-Content -Path $file.FullName -TotalCount 20",
        "    $classLines = $content | Where-Object { $_ -match \"^class \" }",
        "    ",
        "    if ($classLines) {",
        "        Write-Host \"    包含的类:\" -ForegroundColor Gray",
        "        foreach ($line in $classLines) {",
        "            Write-Host \"      $line\" -ForegroundColor Cyan",
        "        }",
        "    }",
        "}",
        "",
        "# 分析导入语句和依赖关系",
        "Write-Host \"`n===== 依赖关系分析 =====\" -ForegroundColor Cyan",
        "foreach ($file in $templateFiles) {",
        "    Write-Host \"`n文件: $($file.Name)\" -ForegroundColor Yellow",
        "    $content = Get-Content -Path $file.FullName",
        "    $importLines = $content | Where-Object { $_ -match \"^import \" -or $_ -match \"^from \" }",
        "    ",
        "    if ($importLines) {",
        "        Write-Host \"  导入语句:\" -ForegroundColor Gray",
        "        foreach ($line in $importLines) {",
        "            Write-Host \"    $line\" -ForegroundColor White",
        "        }",
        "    }",
        "}",
        "",
        "Write-Host \"`n===== 修改建议 =====\" -ForegroundColor Cyan",
        "Write-Host \"1. 创建新模板: 复制现有模板并修改\" -ForegroundColor Green",
        "Write-Host \"2. 扩展现有模板: 创建子类继承现有模板\" -ForegroundColor Green",
        "Write-Host \"3. 修改模板基类: 更新所有模板共享的功能\" -ForegroundColor Green"
    ]
    
    with open(path, 'w', encoding='utf-8') as f:
        f.write("\n".join(script_content))
    
    print(f"已创建分析脚本: {path}")

def import_template(file_path):
    """从JSON文件导入模板到数据库"""
    try:
        # 读取JSON文件
        with open(file_path, 'r', encoding='utf-8') as f:
            content = f.read()
            # 移除可能导致问题的BOM标记
            if (content.startswith('\ufeff')):
                content = content[1:]
            template_data = json.loads(content)
        
        # 连接数据库
        conn = pymysql.connect(**db_config)
        cursor = conn.cursor()
        
        # 检查模板是否已存在
        cursor.execute("SELECT id FROM canvas_template WHERE id = %s", (template_data["id"],))
        exists = cursor.fetchone()
        
        # 准备DSL数据 - 确保是JSON字符串格式
        dsl_json = json.dumps(template_data.get("dsl", {"reference": []}))
        
        # 获取avatar字段（如果存在）
        avatar = template_data.get("avatar", "")
        
        if exists:
            # 更新现有模板
            sql = """
            UPDATE canvas_template 
            SET title = %s, description = %s, canvas_type = %s, dsl = %s, avatar = %s
            WHERE id = %s
            """
            cursor.execute(sql, (
                template_data["title"], 
                template_data["description"], 
                template_data.get("canvas_type", "chatbot"),
                dsl_json,
                avatar,
                template_data["id"]
            ))
            print(f"已更新模板: {template_data['title']} (ID: {template_data['id']})")
        else:
            # 创建新模板
            sql = """
            INSERT INTO canvas_template (id, title, description, canvas_type, dsl, avatar) 
            VALUES (%s, %s, %s, %s, %s, %s)
            """
            cursor.execute(sql, (
                template_data["id"],
                template_data["title"], 
                template_data["description"], 
                template_data.get("canvas_type", "chatbot"),
                dsl_json,
                avatar
            ))
            print(f"已添加新模板: {template_data['title']} (ID: {template_data['id']})")
        
        # 提交更改并关闭连接
        conn.commit()
        conn.close()
        return True
    except Exception as e:
        print(f"处理文件 {os.path.basename(file_path)} 时出错: {e}")
        return False

def import_all_templates(directory):
    """导入目录中的所有JSON模板文件"""
    success_count = 0
    fail_count = 0
    
    # 检查目录是否存在
    if not os.path.exists(directory):
        print(f"错误: 目录 {directory} 不存在")
        return
    
    # 获取目录中的所有JSON文件
    json_files = [f for f in os.listdir(directory) if f.endswith('.json')]
    
    if not json_files:
        print(f"未在 {directory} 目录中找到JSON文件")
        return
    
    print(f"开始导入 {len(json_files)} 个模板文件...")
    
    # 逐一导入每个JSON文件
    for json_file in json_files:
        file_path = os.path.join(directory, json_file)
        print(f"正在处理: {json_file}...", end=" ")
        
        if import_template(file_path):
            success_count += 1
            print("成功!")
        else:
            fail_count += 1
            print("失败!")
    
    print(f"\n导入完成! 成功: {success_count}, 失败: {fail_count}")

def import_json_template():
    """从JSON文件导入新的agent模板"""
    json_path = input("请输入agent JSON文件路径: ")
    
    if not os.path.exists(json_path):
        print(f"文件不存在: {json_path}")
        return
    
    try:
        import json
        
        # 读取JSON文件
        with open(json_path, 'r', encoding='utf-8') as f:
            template_data = json.load(f)
        
        # 提取模板名称
        if 'name' in template_data:
            template_name = template_data['name']
        else:
            template_name = input("JSON中未找到模板名称，请手动输入: ")
        
        # 规范化模板名称为CamelCase
        template_name = ''.join(word.capitalize() for word in template_name.split('_'))
        file_name = f"{template_name.lower()}_template.py"
        file_path = os.path.join(TEMPLATES_DIR, file_name)
        
        # 检查是否已存在
        if os.path.exists(file_path):
            overwrite = input(f"文件 {file_name} 已存在，是否覆盖? (y/n): ")
            if overwrite.lower() != 'y':
                print("操作已取消")
                return
        
        # 生成模板文件
        generate_template_from_json(template_data, template_name, file_path)
        
        print(f"模板已成功从JSON导入: {file_path}")
        print("请检查生成的模板文件格式与语法是否正确")
        
        # 选择是否验证模板
        validate = input("是否验证生成的模板? (y/n): ")
        if validate.lower() == 'y':
            validate_template(file_path)
        
    except json.JSONDecodeError:
        print("JSON格式错误，请检查文件格式")
    except Exception as e:
        print(f"导入过程中出错: {e}")
        import traceback
        traceback.print_exc()

def generate_template_from_json(data, template_name, file_path):
    """根据JSON数据生成模板文件"""
    # 确定基类
    base_template = "BaseAgentTemplate"  # 默认基类
    if 'extends' in data:
        base_template = data['extends']
    
    # 准备导入语句
    imports = ["from typing import Dict, List, Any, Optional"]
    imports.append(f"from .base_template import {base_template}")
    
    # 准备类定义
    class_def = f"class {template_name}Template({base_template}):"
    
    # 准备类文档字符串
    class_docstring = f'    """{template_name}Template'
    if 'description' in data:
        class_docstring += f" - {data['description']}"
    class_docstring += '"""'
    
    # 准备初始化方法
    init_method = [
        "    def __init__(self, *args, **kwargs):",
        "        super().__init__(*args, **kwargs)",
        f"        self.template_name = '{template_name}'"
    ]
    
    # 添加配置属性
    if 'config' in data:
        for key, value in data['config'].items():
            if isinstance(value, str):
                init_method.append(f"        self.{key} = '{value}'")
            else:
                init_method.append(f"        self.{key} = {value}")
    
    # 准备处理方法
    process_methods = []
    
    if 'methods' in data:
        for method in data['methods']:
            method_name = method.get('name', 'process')
            method_desc = method.get('description', '处理方法')
            process_methods.append(f"    def {method_name}(self, input_data: Dict[str, Any]) -> Dict[str, Any]:")
            process_methods.append(f'        """{method_desc}"""')
            
            if 'implementation' in method:
                lines = method['implementation'].split('\n')
                for line in lines:
                    process_methods.append(f"        {line}")
            else:
                process_methods.append("        # 在这里实现方法逻辑")
                process_methods.append("        return input_data")
            
            process_methods.append("")
    else:
        # 添加默认方法
        process_methods.extend([
            "    def process_input(self, input_data: Dict[str, Any]) -> Dict[str, Any]):",
            '        """处理输入数据"""',
            "        # 在这里添加自定义处理逻辑",
            "        return input_data",
            "",
            "    def generate_response(self, context: Dict[str, Any]) -> Dict[str, Any]:",
            '        """生成响应"""',
            "        # 在这里添加自定义响应生成逻辑",
            "        response = {'status': 'success', 'message': 'Template response'}",
            "        return response",
            ""
        ])
    
    # 组合所有部分
    template_content = [
        "# -*- coding: utf-8 -*-",
        '"""',
        f"{template_name} Template - Agent模板",
        ""
    ]
    
    if 'description' in data:
        template_content.append(data['description'])
    
    template_content.extend([
        '"""',
        ""
    ])
    
    template_content.extend(imports)
    template_content.append("")
    template_content.append(class_def)
    template_content.append(class_docstring)
    template_content.append("")
    template_content.extend(init_method)
    template_content.append("")
    template_content.extend(process_methods)
    
    # 写入文件
    with open(file_path, 'w', encoding='utf-8') as f:
        f.write("\n".join(template_content))

def validate_template(file_path):
    """验证生成的模板文件"""
    print(f"验证模板文件: {file_path}")
    
    # 语法检查
    try:
        with open(file_path, 'r', encoding='utf-8') as f:
            source_code = f.read()
        compile(source_code, file_path, 'exec')
        print("✓ 语法检查通过")
    except SyntaxError as e:
        print(f"✗ 语法错误: {e}")
        return False
    
    # 尝试导入模块
    try:
        module_name = os.path.basename(file_path).replace('.py', '')
        spec = importlib.util.spec_from_file_location(module_name, file_path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        print("✓ 模块加载成功")
        
        # 检查类和方法
        template_classes = []
        for name in dir(module):
            item = getattr(module, name)
            if isinstance(item, type) and not name.startswith('__'):
                template_classes.append((name, item))
        
        if template_classes:
            print(f"✓ 找到 {len(template_classes)} 个模板类")
            for class_name, cls in template_classes:
                print(f"  - 类: {class_name}")
                
                # 检查必要方法
                required_methods = ['__init__']
                for method in required_methods:
                    if hasattr(cls, method):
                        print(f"    ✓ 方法: {method}")
                    else:
                        print(f"    ✗ 缺少必要方法: {method}")
        else:
            print("✗ 未找到模板类")
            return False
        
        print("模板验证完成")
        return True
    except Exception as e:
        print(f"✗ 模板验证失败: {e}")
        import traceback
        traceback.print_exc()
        return False

def main():
    """主函数"""
    while True:
        print("\n===== Agent Template 管理工具 =====")
        print("1. 分析现有模板")
        print("2. 修改现有模板")
        print("3. 创建新模板")
        print("4. 运行详细模板分析器")
        print("5. 从JSON文件导入模板")  # 新增选项
        print("0. 退出")
        
        try:
            choice = int(input("\n请选择操作 (输入序号): "))
            
            if choice == 1:
                analyze_templates()
            elif choice == 2:
                modify_template()
            elif choice == 3:
                create_new_template()
            elif choice == 4:
                run_analyzer_script()
            elif choice == 5:
                import_json_template()  # 新增功能
            elif choice == 0:
                print("退出程序")
                break
            else:
                print("无效选择")
                
        except ValueError:
            print("输入无效，请输入数字")
        except Exception as e:
            print(f"发生错误: {e}")
            import traceback
            traceback.print_exc()

if __name__ == "__main__":
    main()