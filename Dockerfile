# base stage
FROM ubuntu:24.04 AS base
USER root

ARG ARCH=amd64
ENV LIGHTEN=0

WORKDIR /ragflow

RUN rm -f /etc/apt/apt.conf.d/docker-clean \
    && echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' > /etc/apt/apt.conf.d/keep-cache

RUN --mount=type=cache,id=ragflow_base_apt,target=/var/cache/apt,sharing=locked \
    apt update && apt-get --no-install-recommends install -y ca-certificates

# If you are at mainland China, you can use tsinghua mirror to speed up apt and poetry
RUN sed -i 's|http://archive.ubuntu.com|https://mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list.d/ubuntu.sources
ENV POETRY_PYPI_MIRROR_URL=https://pypi.tuna.tsinghua.edu.cn/simple/

RUN --mount=type=cache,id=ragflow_base_apt,target=/var/cache/apt,sharing=locked \
    apt update && apt install -y curl libpython3-dev nginx libglib2.0-0 libglx-mesa0 pkg-config libicu-dev libgdiplus python3-pip python3-poetry \
    && pip3 install --user --break-system-packages poetry-plugin-pypi-mirror --index-url https://pypi.tuna.tsinghua.edu.cn/simple/ \
    && rm -rf /var/lib/apt/lists/*

# https://forum.aspose.com/t/aspose-slides-for-net-no-usable-version-of-libssl-found-with-linux-server/271344/13
# aspose-slides on linux/arm64 is unavailable
RUN if [ "${ARCH}" = "amd64" ]; then \
        curl -o libssl1.deb http://archive.ubuntu.com/ubuntu/pool/main/o/openssl/libssl1.1_1.1.1f-1ubuntu2_amd64.deb && dpkg -i libssl1.deb && rm -f libssl1.deb; \
    fi

ENV PYTHONDONTWRITEBYTECODE=1 DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1

# Configure Poetry
ENV POETRY_NO_INTERACTION=1
ENV POETRY_VIRTUALENVS_IN_PROJECT=true
ENV POETRY_VIRTUALENVS_CREATE=true
ENV POETRY_REQUESTS_TIMEOUT=15

# builder stage
FROM base AS builder
USER root

WORKDIR /ragflow

RUN --mount=type=cache,id=ragflow_builder_apt,target=/var/cache/apt,sharing=locked \
    apt update && apt install -y nodejs npm cargo && \
    rm -rf /var/lib/apt/lists/*

COPY web web
RUN --mount=type=cache,id=ragflow_builder_npm,target=/root/.npm,sharing=locked \
    cd web && npm i --force && npm run build

# install dependencies from poetry.lock file
COPY pyproject.toml poetry.toml poetry.lock ./

RUN --mount=type=cache,id=ragflow_builder_poetry,target=/root/.cache/pypoetry,sharing=locked \
    if [ "$LIGHTEN" -eq 0 ]; then \
        poetry install --sync --no-root --with=full; \
    else \
        poetry install --sync --no-root; \
    fi

# production stage
FROM base AS production
USER root

WORKDIR /ragflow

# Install python packages' dependencies
# cv2 requires libGL.so.1
RUN --mount=type=cache,id=ragflow_production_apt,target=/var/cache/apt,sharing=locked \
    apt update && apt install -y --no-install-recommends nginx libgl1 vim less && \
    rm -rf /var/lib/apt/lists/*

COPY web web
COPY api api
COPY conf conf
COPY deepdoc deepdoc
COPY rag rag
COPY agent agent
COPY graphrag graphrag
COPY pyproject.toml poetry.toml poetry.lock ./

# Copy models downloaded via download_deps.py
RUN mkdir -p /ragflow/rag/res/deepdoc /root/.ragflow
RUN --mount=type=bind,source=huggingface.co,target=/huggingface.co \
    tar --exclude='.*' -cf - \
        /huggingface.co/InfiniFlow/text_concat_xgb_v1.0 \
        /huggingface.co/InfiniFlow/deepdoc \
        | tar -xf - --strip-components=3 -C /ragflow/rag/res/deepdoc
RUN --mount=type=bind,source=huggingface.co,target=/huggingface.co \
    tar -cf - \
        /huggingface.co/BAAI/bge-large-zh-v1.5 \
        /huggingface.co/BAAI/bge-reranker-v2-m3 \
        /huggingface.co/maidalun1020/bce-embedding-base_v1 \
        /huggingface.co/maidalun1020/bce-reranker-base_v1 \
        | tar -xf - --strip-components=2 -C /root/.ragflow

# Copy nltk data downloaded via download_deps.py
COPY nltk_data /root/nltk_data

# Copy compiled web pages
COPY --from=builder /ragflow/web/dist /ragflow/web/dist

# Copy Python environment and packages
ENV VIRTUAL_ENV=/ragflow/.venv
COPY --from=builder ${VIRTUAL_ENV} ${VIRTUAL_ENV}
ENV PATH="${VIRTUAL_ENV}/bin:${PATH}"

ENV PYTHONPATH=/ragflow/

COPY docker/entrypoint.sh ./entrypoint.sh
RUN chmod +x ./entrypoint.sh

ENTRYPOINT ["./entrypoint.sh"]
