import os
import sys
import json
import logging
import time
from elasticsearch import Elasticsearch
from opensearchpy import OpenSearch
from dotenv import load_dotenv

# Add project root to path to import ragflow modules if needed
sys.path.append(os.path.join(os.path.dirname(__file__), '..'))

logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)

def migrate():
    # Load environment variables from docker/.env
    env_path = os.path.join(os.path.dirname(__file__), '..', 'docker', '.env')
    if os.path.exists(env_path):
        logger.info(f"Loading env from {env_path}")
        load_dotenv(env_path)
    else:
        logger.warning(f"Env file not found at {env_path}, using defaults.")

    es_host = os.getenv('ES_HOST', 'localhost')
    es_port = os.getenv('ES_PORT', '1200')
    es_user = 'elastic'
    es_password = os.getenv('ELASTIC_PASSWORD', 'infini_rag_flow')

    os_host = os.getenv('OS_HOST', 'localhost')
    os_port = os.getenv('OS_PORT', '1201')
    os_user = os.getenv('OS_USER', 'admin')
    os_password = os.getenv('OPENSEARCH_PASSWORD', 'infini_rag_flow_OS_01')

    # Connection URLs
    # If running from host machine, use localhost and mapped ports
    es_url = f"http://localhost:{es_port}"
    os_url = f"http://localhost:{os_port}"

    logger.info(f"Connecting to Elasticsearch at {es_url}")
    try:
        es = Elasticsearch(es_url, basic_auth=(es_user, es_password))
        if not es.ping():
            logger.error("Could not ping Elasticsearch. Make sure it's running and accessible.")
            return
    except Exception as e:
        logger.error(f"Failed to connect to ES: {e}")
        return
    
    logger.info(f"Connecting to OpenSearch at {os_url}")
    try:
        os_client = OpenSearch(os_url, http_auth=(os_user, os_password), verify_certs=False)
        if not os_client.ping():
            logger.error("Could not ping OpenSearch. Make sure it's running and accessible.")
            return
    except Exception as e:
        logger.error(f"Failed to connect to OS: {e}")
        return

    # 1. Get all ragflow indices from ES
    try:
        indices_info = es.indices.get(index="ragflow*")
        indices = list(indices_info.keys())
    except Exception as e:
        logger.error(f"Failed to get indices from ES: {e}")
        return

    if not indices:
        logger.info("No ragflow indices found in Elasticsearch.")
        return

    logger.info(f"Found {len(indices)} indices to migrate: {indices}")

    # Load mappings
    mapping_path = os.path.join(os.path.dirname(__file__), '..', 'conf', 'os_mapping.json')
    msg_mapping_path = os.path.join(os.path.dirname(__file__), '..', 'conf', 'mapping.json')
    
    os_mapping = None
    if os.path.exists(mapping_path):
        with open(mapping_path, 'r') as f:
            os_mapping = json.load(f)
    
    msg_mapping = None
    if os.path.exists(msg_mapping_path):
        with open(msg_mapping_path, 'r') as f:
            msg_mapping = json.load(f)

    for index_name in indices:
        logger.info(f"--- Migrating index: {index_name} ---")

        # 2. Create index in OpenSearch if not exists
        if not os_client.indices.exists(index=index_name):
            logger.info(f"Creating index {index_name} in OpenSearch...")
            
            # Determine which mapping to use
            current_mapping = os_mapping
            if "_msg_" in index_name:
                current_mapping = msg_mapping
                logger.info(f"Using message mapping for {index_name}")
            else:
                logger.info(f"Using document mapping for {index_name}")
                
            if current_mapping:
                os_client.indices.create(index=index_name, body=current_mapping)
            else:
                logger.warning(f"No mapping file found, creating index with default settings.")
                os_client.indices.create(index=index_name)
        else:
            logger.info(f"Index {index_name} already exists in OpenSearch.")

        # 3. Trigger Reindex from Remote
        # We use the internal docker name 'es01' because OpenSearch container will pull from it
        reindex_body = {
            "source": {
                "remote": {
                    "host": "http://es01:9200",
                    "username": es_user,
                    "password": es_password
                },
                "index": index_name
            },
            "dest": {
                "index": index_name
            }
        }
        
        try:
            logger.info(f"Starting reindex task for {index_name}...")
            # We use wait_for_completion=False to avoid timing out on large indices
            res = os_client.reindex(body=reindex_body, wait_for_completion=False)
            task_id = res.get('task')
            logger.info(f"Reindex task started for {index_name}. Task ID: {task_id}")
        except Exception as e:
            logger.error(f"Failed to start reindex for {index_name}: {e}")
            logger.error("Check if 'reindex.remote.whitelist' is set in OpenSearch configuration.")

    logger.info("--- Migration Summary ---")
    logger.info("All migration tasks have been submitted.")
    logger.info("You can monitor the status of tasks using:")
    logger.info(f"GET {os_url}/_tasks?actions=*reindex&detailed=true")
    logger.info("Or visit OpenSearch Dashboards (Dev Tools).")

if __name__ == "__main__":
    migrate()
