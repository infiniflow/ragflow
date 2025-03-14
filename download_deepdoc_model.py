from huggingface_hub import snapshot_download

# 指定模型名称
model_name = "InfiniFlow/deepdoc"

# 下载模型到本地目录
save_directory = "/root/software/ragflow/rag/res/deepdoc"
snapshot_download(repo_id=model_name, local_dir=save_directory)

print(f"模型已下载并保存到 {save_directory}")