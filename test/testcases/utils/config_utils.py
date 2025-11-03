DEFAULT_PARSER_CONFIG = {
    "layout_recognize": "DeepDOC",
    "chunk_token_num": 512,
    "delimiter": "\n",
    "auto_keywords": 0,
    "auto_questions": 0,
    "html4excel": False,
    "topn_tags": 3,
    "raptor": {
        "use_raptor": True,
        "prompt": "Please summarize the following paragraphs. Be careful with the numbers, do not make things up. Paragraphs as following:\n      {cluster_content}\nThe above is the content you need to summarize.",
        "max_token": 256,
        "threshold": 0.1,
        "max_cluster": 64,
        "random_seed": 0,
    },
    "graphrag": {
        "use_graphrag": True,
        "entity_types": [
            "organization",
            "person",
            "geo",
            "event",
            "category",
        ],
        "method": "light",
    },
}