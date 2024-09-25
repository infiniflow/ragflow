---
sidebar_position: 2
slug: /launch_ragflow_from_source
---

# Launch the RAGFlow Service from Source 

A guide explaining how to set up a RAGFlow service from its source code. By following this guide, you'll be equipped to debug using the source code.

## Target Audience

Developers who have added new features or modified existing code and wish to debug using the source code, *provided that* their machine has the target deployment environment set up.

## Prerequisites

- CPU &ge; 4 cores
- RAM &ge; 16 GB
- Disk &ge; 50 GB
- Docker &ge; 24.0.0 & Docker Compose &ge; v2.26.1

:::tip NOTE
If you have not installed Docker on your local machine (Windows, Mac, or Linux), see the [Install Docker Engine](https://docs.docker.com/engine/install/) guide.
:::

## Launch the Service from Source

To launch the RAGFlow service from source code:

### 1. Clone the RAGFlow Repository

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow/
```

### 2. Install Python dependencies

1. Install Poetry:
   
   ```bash
   curl -sSL https://install.python-poetry.org | python3 -
   ```

2. Configure Poetry:

   ```bash
   export POETRY_VIRTUALENVS_CREATE=true POETRY_VIRTUALENVS_IN_PROJECT=true
   ```

3. Install all Python dependencies:

   ```bash
   ~/.local/bin/poetry install --sync --no-root
   ```
   *A virtual environment named `.venv` is created, and all Python dependencies are installed into the new environment*

### 3. Launch Third-party Services

The following command launches the 'base' services (MinIO, Elasticsearch, Redis, and MySQL) using Docker Compose:

```bash
docker compose -f docker/docker-compose-base.yml up -d
```

### 4. Update `host` and `port` Settings for Third-party Services

1. Add the following line to `/etc/hosts` to resolve all hosts in **docker/service_conf.yaml** to `127.0.0.1`:

   ```
   127.0.0.1       es01 mysql minio redis
   ```

2. In **docker/service_conf.yaml**, update mysql port to `5455` and es port to `1200`, as specified in **docker/.env**.

3. Replace **conf/service_conf.yaml** with the contents of **docker/service_conf.yaml**.

### 5. Launch the RAGFlow Backend Service

1. Comment out the `nginx` line in **docker/entrypoint.sh**.

   ```
   # /usr/sbin/nginx
   ```

2. Activate Python virtual env

   ```bash
   source .venv/bin/activate
   export PYTHONPATH=$(pwd)
   ```

3.  [Optional] If you are inside GFW
 
   ```bash
   export HF_ENDPOINT=https://hf-mirror.com
   ```

4. Run the **entrypoint.sh** script to launch the backend service:

   ```
   bash docker/entrypoint.sh
   ```

### 6. Launch the frontend service

1. Navigate to the web directory:

   ```bash
   cd web
   ```

2. Install frontend dependencies:

   ```bash
   npm install --force
   ```

3. Update the proxy configuration in **.umirc.ts**:

   ```bash
   vim .umirc.ts
   ```

   Update proxy.target to http://127.0.0.1:9380

4. Start up the frontend service:

   ```bash
   npm run dev 
   ```

### 7. Access the RAGFlow service

In your web browser, enter `http://127.0.0.1/`.