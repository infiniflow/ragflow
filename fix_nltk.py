import nltk.data
import os
import shutil

# 获取NLTK数据路径
data_paths = [p for p in nltk.data.path if os.path.exists(p)]
if not data_paths:
    print("No valid NLTK data paths found")
    exit(1)

nltk_data_path = data_paths[0]
print(f"Using NLTK data path: {nltk_data_path}")

# 清理损坏的punkt_tab目录
punkt_tab_path = os.path.join(nltk_data_path, 'tokenizers', 'punkt_tab')
if os.path.exists(punkt_tab_path):
    print('Removing corrupted punkt_tab directory...')
    try:
        shutil.rmtree(punkt_tab_path)
        print('Removed successfully')
    except Exception as e:
        print(f'Error removing directory: {e}')

# 下载标准的punkt资源
print('Downloading standard punkt resource...')
try:
    nltk.download('punkt')
    print('Download complete!')
except Exception as e:
    print(f'Error downloading punkt: {e}')

# 尝试将punkt链接到punkt_tab
print('Linking punkt to punkt_tab...')
punkt_path = os.path.join(nltk_data_path, 'tokenizers', 'punkt')
if os.path.exists(punkt_path) and not os.path.exists(punkt_tab_path):
    try:
        # 在Windows上创建目录的副本
        shutil.copytree(punkt_path, punkt_tab_path)
        print('Created punkt_tab directory from punkt')
    except Exception as e:
        print(f'Error creating punkt_tab directory: {e}')

print('Fix complete!')
