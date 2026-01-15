FROM python:3.11-slim-bookworm

RUN grep -rl 'deb.debian.org' /etc/apt/ | xargs sed -i 's|http[s]*://deb.debian.org|https://mirrors.tuna.tsinghua.edu.cn|g' && \
    apt-get update && \
    apt-get install -y curl gcc && \
    rm -rf /var/lib/apt/lists/*

ARG TARGETARCH
ARG TARGETVARIANT

RUN set -eux; \
    case "${TARGETARCH}${TARGETVARIANT}" in \
        amd64) DOCKER_ARCH=x86_64 ;; \
        arm64) DOCKER_ARCH=aarch64 ;; \
        armv7) DOCKER_ARCH=armhf ;; \
        armv6) DOCKER_ARCH=armel ;; \
        arm64v8) DOCKER_ARCH=aarch64 ;; \
        arm64v7) DOCKER_ARCH=armhf ;; \
        arm*) DOCKER_ARCH=armhf ;; \
        ppc64le) DOCKER_ARCH=ppc64le ;; \
        s390x) DOCKER_ARCH=s390x ;; \
        *) echo "Unsupported architecture: ${TARGETARCH}${TARGETVARIANT}" && exit 1 ;; \
    esac; \
    echo "Downloading Docker for architecture: ${DOCKER_ARCH}"; \
    curl -fsSL "https://download.docker.com/linux/static/stable/${DOCKER_ARCH}/docker-29.1.0.tgz" | \
    tar xz -C /usr/local/bin --strip-components=1 docker/docker; \
    ln -sf /usr/local/bin/docker /usr/bin/docker

COPY --from=ghcr.io/astral-sh/uv:0.7.5 /uv /uvx /bin/
ENV UV_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple

WORKDIR /app
COPY . .

RUN uv pip install --system -r requirements.txt

CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "9385"]
