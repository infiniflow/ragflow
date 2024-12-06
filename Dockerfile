# base stage
FROM ubuntu:22.04 AS base
USER root
SHELL ["/bin/bash", "-c"]

ARG LIGHTEN=0
ENV LIGHTEN=${LIGHTEN}

WORKDIR /ragflow

# Copy models downloaded via download_deps.py
RUN mkdir -p /ragflow/rag/res/deepdoc /root/.ragflow
RUN --mount=type=bind,from=infiniflow/ragflow_deps:latest,source=/huggingface.co,target=/huggingface.co \
    tar --exclude='.*' -cf - \
        /huggingface.co/InfiniFlow/text_concat_xgb_v1.0 \
        /huggingface.co/InfiniFlow/deepdoc \
        | tar -xf - --strip-components=3 -C /ragflow/rag/res/deepdoc
RUN --mount=type=bind,from=infiniflow/ragflow_deps:latest,source=/huggingface.co,target=/huggingface.co \
    if [ "$LIGHTEN" == "0" ]; then \
        (tar -cf - \
            /huggingface.co/BAAI/bge-large-zh-v1.5 \
            /huggingface.co/BAAI/bge-reranker-v2-m3 \
            /huggingface.co/maidalun1020/bce-embedding-base_v1 \
            /huggingface.co/maidalun1020/bce-reranker-base_v1 \
            | tar -xf - --strip-components=2 -C /root/.ragflow) \
    fi

# https://github.com/chrismattmann/tika-python
# This is the only way to run python-tika without internet access. Without this set, the default is to check the tika version and pull latest every time from Apache.
RUN --mount=type=bind,from=infiniflow/ragflow_deps:latest,source=/,target=/deps \
    cp -r /deps/nltk_data /root/ && \
    cp /deps/tika-server-standard-3.0.0.jar /deps/tika-server-standard-3.0.0.jar.md5 /ragflow/ && \
    cp /deps/cl100k_base.tiktoken /ragflow/9b5ad71b2ce5302211f9c61530b329a4922fc6a4

ENV TIKA_SERVER_JAR="file:///ragflow/tika-server-standard-3.0.0.jar"

# Setup apt
RUN --mount=type=cache,id=ragflow_apt,target=/var/cache/apt,sharing=locked \
    sed -i 's|http://archive.ubuntu.com|https://mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list && \
    rm -f /etc/apt/apt.conf.d/docker-clean && \
    echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' > /etc/apt/apt.conf.d/keep-cache && \
    apt update && apt --no-install-recommends install -y ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# cv2 requires libGL.so.1
RUN --mount=type=cache,id=ragflow_apt,target=/var/cache/apt,sharing=locked \
    apt update && DEBIAN_FRONTEND=noninteractive apt install -y curl libpython3-dev nginx libglib2.0-0 libglx-mesa0 pkg-config libicu-dev libgdiplus default-jdk python3-pip pipx \
    libatk-bridge2.0-0 libgtk-4-1 libnss3 xdg-utils unzip libgbm-dev wget git nginx libgl1 vim less && \
    rm -rf /var/lib/apt/lists/*

RUN pip3 config set global.index-url https://pypi.tuna.tsinghua.edu.cn/simple && \
    pip3 config set global.trusted-host pypi.tuna.tsinghua.edu.cn && \
    pipx install poetry && \
    pipx runpip poetry config set global.index-url https://pypi.tuna.tsinghua.edu.cn/simple && \
    pipx runpip poetry config set global.trusted-host pypi.tuna.tsinghua.edu.cn && \
    /root/.local/bin/poetry self add poetry-plugin-pypi-mirror

ENV PYTHONDONTWRITEBYTECODE=1 DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1
ENV PATH=/root/.local/bin:$PATH
# Configure Poetry
ENV POETRY_NO_INTERACTION=1
ENV POETRY_VIRTUALENVS_IN_PROJECT=true
ENV POETRY_VIRTUALENVS_CREATE=true
ENV POETRY_REQUESTS_TIMEOUT=15
ENV POETRY_PYPI_MIRROR_URL=https://pypi.tuna.tsinghua.edu.cn/simple/

# nodejs 12.22 on Ubuntu 22.04 is too old
RUN --mount=type=cache,id=ragflow_apt,target=/var/cache/apt,sharing=locked \
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash - && \
    apt purge -y nodejs npm && \
    apt autoremove && \
    apt update && \
    apt install -y nodejs cargo && \
    rm -rf /var/lib/apt/lists/*

# Add dependencies of selenium
RUN --mount=type=bind,from=infiniflow/ragflow_deps:latest,source=/chrome-linux64-121-0-6167-85,target=/chrome-linux64.zip \
    unzip /chrome-linux64.zip && \
    mv chrome-linux64 /opt/chrome && \
    ln -s /opt/chrome/chrome /usr/local/bin/
RUN --mount=type=bind,from=infiniflow/ragflow_deps:latest,source=/chromedriver-linux64-121-0-6167-85,target=/chromedriver-linux64.zip \
    unzip -j /chromedriver-linux64.zip chromedriver-linux64/chromedriver && \
    mv chromedriver /usr/local/bin/ && \
    rm -f /usr/bin/google-chrome

# https://forum.aspose.com/t/aspose-slides-for-net-no-usable-version-of-libssl-found-with-linux-server/271344/13
# aspose-slides on linux/arm64 is unavailable
RUN --mount=type=bind,from=infiniflow/ragflow_deps:latest,source=/,target=/deps \
    if [ "$(uname -m)" = "x86_64" ]; then \
        dpkg -i /deps/libssl1.1_1.1.1f-1ubuntu2_amd64.deb; \
    elif [ "$(uname -m)" = "aarch64" ]; then \
        dpkg -i /deps/libssl1.1_1.1.1f-1ubuntu2_arm64.deb; \
    fi


# builder stage
FROM base AS builder
USER root

WORKDIR /ragflow

# install dependencies from poetry.lock file
COPY pyproject.toml poetry.toml poetry.lock ./

RUN --mount=type=cache,id=ragflow_poetry,target=/root/.cache/pypoetry,sharing=locked \
    if [ "$LIGHTEN" == "1" ]; then \
        poetry install --no-root; \
    else \
        poetry install --no-root --with=full; \
    fi

COPY web web
COPY docs docs
RUN --mount=type=cache,id=ragflow_npm,target=/root/.npm,sharing=locked \
    cd web && npm install --force && npm run build

COPY .git /ragflow/.git

RUN version_info=$(git describe --tags --match=v* --dirty); \
    if [ "$LIGHTEN" == "1" ]; then \
        version_info="$version_info slim"; \
    else \
        version_info="$version_info full"; \
    fi; \
    echo "RAGFlow version: $version_info"; \
    echo $version_info > /ragflow/VERSION

# production stage
FROM base AS production
USER root

WORKDIR /ragflow

# Copy Python environment and packages
ENV VIRTUAL_ENV=/ragflow/.venv
COPY --from=builder ${VIRTUAL_ENV} ${VIRTUAL_ENV}
ENV PATH="${VIRTUAL_ENV}/bin:${PATH}"

ENV PYTHONPATH=/ragflow/

COPY web web
COPY api api
COPY conf conf
COPY deepdoc deepdoc
COPY rag rag
COPY agent agent
COPY graphrag graphrag
COPY pyproject.toml poetry.toml poetry.lock ./

COPY docker/service_conf.yaml.template ./conf/service_conf.yaml.template
COPY docker/entrypoint.sh ./entrypoint.sh
RUN chmod +x ./entrypoint.sh

# Copy compiled web pages
COPY --from=builder /ragflow/web/dist /ragflow/web/dist

COPY --from=builder /ragflow/VERSION /ragflow/VERSION

ENTRYPOINT ["./entrypoint.sh"]
