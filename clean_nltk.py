import os
import shutil

# NLTK数据路径
nltk_data_path = r"C:\Users\xyfig\AppData\Roaming\nltk_data\tokenizers"

# 要删除的损坏文件
files_to_remove = [
    os.path.join(nltk_data_path, "punkt_tab.zip"),
    os.path.join(nltk_data_path, "punkt.zip")
]

print(f"Cleaning NLTK data in: {nltk_data_path}")

for file_path in files_to_remove:
    if os.path.exists(file_path):
        try:
            os.remove(file_path)
            print(f"Removed: {file_path}")
        except Exception as e:
            print(f"Failed to remove {file_path}: {e}")
    else:
        print(f"File not found: {file_path}")

# 检查是否有punkt目录需要清理
punkt_dir = os.path.join(nltk_data_path, "punkt")
if os.path.exists(punkt_dir):
    try:
        shutil.rmtree(punkt_dir)
        print(f"Removed directory: {punkt_dir}")
    except Exception as e:
        print(f"Failed to remove directory {punkt_dir}: {e}")

punkt_tab_dir = os.path.join(nltk_data_path, "punkt_tab")
if os.path.exists(punkt_tab_dir):
    try:
        shutil.rmtree(punkt_tab_dir)
        print(f"Removed directory: {punkt_tab_dir}")
    except Exception as e:
        print(f"Failed to remove directory {punkt_tab_dir}: {e}")

print("Cleanup complete!")
