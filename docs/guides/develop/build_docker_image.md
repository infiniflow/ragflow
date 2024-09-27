---
sidebar_position: 1
slug: /build_docker_image
---

# Build a RAGFlow Docker Image

A guide explaining how to build a RAGFlow Docker image from its source code. By following this guide, you'll be able to create a local Docker image that can be used for development, debugging, or testing purposes.

## Target Audience

- Developers who have added new features or modified the existing code and require a Docker image to view and debug their changes.
- Testers looking to explore the latest features of RAGFlow in a Docker image.

## Prerequisites

- CPU &ge; 4 cores
- RAM &ge; 16 GB
- Disk &ge; 50 GB
- Docker &ge; 24.0.0 & Docker Compose &ge; v2.26.1

:::tip NOTE
If you have not installed Docker on your local machine (Windows, Mac, or Linux), see the [Install Docker Engine](https://docs.docker.com/engine/install/) guide.
:::

## Build a RAGFlow Docker Image

To build a RAGFlow Docker image from source code:

### Git Clone the Repository

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow
```

### Build the Docker Image

Navigate to the `ragflow` directory where the Dockerfile and other necessary files are located. Now you can build the Docker image using the provided Dockerfile. The command below specifies which Dockerfile to use and tages the image with a name for reference purpose.

#### Build image `ragflow:dev-slim`
```bash
docker build -f Dockerfile.slim -t infiniflow/ragflow:dev-slim .
```
This image's size is about 1GB. It relies external LLM services since it doesn't contain embedding models. 

#### Build image `ragflow:dev`
```bash
cd ragflow/
docker build -f Dockerfile -t infiniflow/ragflow:dev .
```
This image's size is about 11GB. It contains embedding models, and can inference via local CPU/GPU or external LLM services.
