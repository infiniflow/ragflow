import sys
files = [
    "api/apps/restful_apis/bot_api.py",
    "api/apps/restful_apis/chunk_api.py",
    "api/apps/restful_apis/dify_retrieval_api.py",
    "api/apps/services/dataset_api_service.py",
    "common/metadata_utils.py",
    "rag/nlp/search.py"
]

for f in files:
    print(f"--- {f} ---")
    with open(f, "r") as file:
        lines = file.readlines()
        in_conflict = False
        for i, line in enumerate(lines):
            if line.startswith("<<<<<<< HEAD"):
                in_conflict = True
                print(f"Conflict at line {i+1}:")
            if in_conflict:
                print(line, end="")
            if line.startswith(">>>>>>>"):
                in_conflict = False
                print("\n")
