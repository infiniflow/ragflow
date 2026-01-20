
---

### (1). Deploy RAGFlow services and images

[https://ragflow.io/docs/build_docker_image](https://ragflow.io/docs/build_docker_image)

### (2). Configure the required environment for testing

**Install Python dependencies (including test dependencies):**

```bash
uv sync --python 3.12 --only-group test --no-default-groups --frozen

```

**Activate the environment:**

```bash
source .venv/bin/activate

```

**Install SDK:**

```bash
uv pip install sdk/python 

```

**Modify the `.env` file:** Add the following code:

```env
COMPOSE_PROFILES=${COMPOSE_PROFILES},tei-cpu
TEI_MODEL=BAAI/bge-small-en-v1.5
RAGFLOW_IMAGE=infiniflow/ragflow:v0.23.1 #Replace with the image you are using

```

**Start the container（wait two minutes）:**

```bash
docker compose -f docker/docker-compose.yml up -d

```

---

### (3). Test Elasticsearch

**a) Run sdk tests against Elasticsearch:**

```bash
export HTTP_API_TEST_LEVEL=p2
export HOST_ADDRESS=http://127.0.0.1:9380  # Ensure that this port is the API port mapped to your localhost
pytest -s --tb=short --level=${HTTP_API_TEST_LEVEL} test/testcases/test_sdk_api 

```

**b) Run http api tests against Elasticsearch:**

```bash
pytest -s --tb=short --level=${HTTP_API_TEST_LEVEL} test/testcases/test_http_api 

```

---

### (4). Test Infinity

**Modify the `.env` file:**

```env
DOC_ENGINE=${DOC_ENGINE:-infinity}

```

**Start the container:**

```bash
docker compose -f docker/docker-compose.yml down -v 
docker compose -f docker/docker-compose.yml up -d

```

**a) Run sdk tests against Infinity:**

```bash
DOC_ENGINE=infinity pytest -s --tb=short --level=${HTTP_API_TEST_LEVEL} test/testcases/test_sdk_api 

```

**b) Run http api tests against Infinity:**

```bash
DOC_ENGINE=infinity pytest -s --tb=short --level=${HTTP_API_TEST_LEVEL} test/testcases/test_http_api 

```