---
sidebar_position: 3
title: Launch Service from Source
sidebar_label: Launch Service from Source
slug: /launch_ragflow_from_source
sidebar_custom_props: {
  categoryIcon: LucideMonitorPlay
}
---
# Launch Service from Source

A guide explaining how to set up a RAGFlow service from its source code. By following this guide, you'll be able to debug using the source code.

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

## Launch a Service from Source

To launch a RAGFlow service from source code:

### Clone the RAGFlow Repository

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow/
```

### Install Go and Native Dependencies

1. Install Go:

   ```bash
   wget https://go.dev/dl/go1.26.4.linux-amd64.tar.gz
   sudo rm -rf /usr/local/go
   sudo tar -C /usr/local -xzf go1.25.4.linux-amd64.tar.gz
   echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
   source ~/.bashrc
   go version
   ```

2. Install native dependencies:

   sudo apt update
   sudo apt install -y cmake clang-20 lld-20 libpcre2-dev

3. Download the native libraries and model files required by Go:

   python3 ragflow_deps/download_go_deps.py

4. Build the Go binaries and the required C++ bindings:

   ```bash
   ./build.sh --all
   ```

   For a production build with debug symbols removed:

   ```bash
   ./build.sh --strip --all
   ```

### Launch Third-Party Services

The following command launches the required services, including MinIO, Infinity, Redis, MySQL, NATS, and Kvrocks, using Docker Compose:

```bash
docker compose -f docker/docker-compose-base.yml --profile ragflow-go --profile infinity --profile mysql up -d
```

### Update `host` and `port` Settings for Third-Party Services

1. Add the following line to `/etc/hosts` to resolve all hosts specified in **docker/service_conf.yaml.template** to `127.0.0.1`:

   ```
   127.0.0.1       es01 infinity mysql minio redis
   ```

2. In **docker/service_conf.yaml.template**, update mysql port to `5455` and es port to `1200`, as specified in **docker/.env**.


### Launch the RAGFlow Go Backend

1. Check the configuration in **conf/service_conf.yaml**, ensuring all hosts and ports are correctly set.

2. Run database migrations:

   ./bin/ragflow_server --migrate

3. Start the admin server:

   ./bin/ragflow_server --admin

4. Open another terminal and start the API server:

   ./bin/ragflow_server --api

5. Open another terminal and start the ingestor:

   ./bin/ragflow_server --ingestor

The admin server must be started before the API and ingestor servers.

### Launch the RAGFlow Frontend Service

1. Navigate to the `web` directory and install the frontend dependencies:

   ```bash
   cd web
   npm install
   ```

2. Start the RAGFlow frontend service with the proxy configured for the Go backend:

   API_PROXY_SCHEME=go npm run dev

The `go` proxy scheme routes all API requests to the Go API server on port `9384`.


### Access the RAGFlow Service

In your web browser, enter `http://127.0.0.1:<PORT>/`, ensuring the port number matches that shown in the screenshot above.

### Stop the RAGFlow Service When the Development Is Done

1. Stop the frontend service by pressing `Ctrl+C` in the frontend terminal.

2. Stop the Go admin, API, and ingestor services by pressing `Ctrl+C` in their respective terminals.

3. Stop the dependency containers:

   docker compose -f docker/docker-compose-base.yml --profile ragflow-go --profile infinity down
