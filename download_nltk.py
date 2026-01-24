import nltk
import os
import sys
import time

def download_with_retry(package_name, max_retries=3):
    """尝试多次下载NLTK资源"""
    for attempt in range(max_retries):
        try:
            print(f"Attempting to download {package_name} (attempt {attempt + 1}/{max_retries})...")
            nltk.download(package_name)
            print(f"Successfully downloaded {package_name}")
            return True
        except Exception as e:
            print(f"Attempt {attempt + 1} failed: {e}")
            if attempt < max_retries - 1:
                print("Retrying in 5 seconds...")
                time.sleep(5)
            else:
                print(f"Failed to download {package_name} after {max_retries} attempts")
                return False

def main():
    print("Starting NLTK resource download...")
    
    # 下载需要的资源
    packages = ['punkt', 'punkt_tab', 'averaged_perceptron_tagger', 'wordnet']
    
    success_count = 0
    for package in packages:
        if download_with_retry(package):
            success_count += 1
    
    print(f"\nDownload summary: {success_count}/{len(packages)} packages downloaded successfully")
    
    if success_count == len(packages):
        print("All NLTK resources downloaded successfully!")
        return 0
    else:
        print("Some NLTK resources failed to download. You may need to download them manually.")
        return 1

if __name__ == "__main__":
    sys.exit(main())
