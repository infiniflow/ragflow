# base stage
FROM ubuntu:24.04 AS base
USER root
SHELL ["/bin/bash", "-c"]

ARG NEED_MIRROR=0

WORKDIR /ragflow

# copy models downloaded via download_deps.py
RUN mkdir -p /ragflow/rag/res/deepdoc /opt/ragflow_home
RUN --mount=type=bind,from=infiniflow/ragflow_deps:latest,source=/huggingface.co,target=/huggingface.co \
    tar --exclude='.*' -cf - \
        /huggingface.co/InfiniFlow/text_concat_xgb_v1.0 \
        /huggingface.co/InfiniFlow/deepdoc \
        | tar -xf - --strip-components=3 -C /ragflow/rag/res/deepdoc

# https://github.com/chrismattmann/tika-python
# This is the only way to run python-tika without internet access. Without this set, the default is to check the tika version and pull latest every time from Apache.
RUN --mount=type=bind,from=infiniflow/ragflow_deps:latest,source=/,target=/deps \
    mkdir -p /opt/nltk_data && \
    cp -r /deps/nltk_data/* /opt/nltk_data/ && \
    cp /deps/tika-server-standard-3.2.3.jar /deps/tika-server-standard-3.2.3.jar.md5 /ragflow/ && \
    cp /deps/cl100k_base.tiktoken /ragflow/9b5ad71b2ce5302211f9c61530b329a4922fc6a4

ENV TIKA_SERVER_JAR="file:///ragflow/tika-server-standard-3.2.3.jar"
ENV DEBIAN_FRONTEND=noninteractive
ENV NLTK_DATA=/opt/nltk_data

# Setup apt
# Python package and implicit dependencies:
# opencv-python: libglib2.0-0 libglx-mesa0 libgl1
# python-pptx:   default-jdk                              tika-server-standard-3.2.3.jar
# selenium:      libatk-bridge2.0-0                       chrome-linux64-121-0-6167-85
# Building C extensions: libpython3-dev libgtk-4-1 libnss3 xdg-utils libgbm-dev
RUN --mount=type=cache,id=ragflow_apt,target=/var/cache/apt,sharing=locked \
    apt update && \
    apt --no-install-recommends install -y ca-certificates; \
    if [ "$NEED_MIRROR" == "1" ]; then \
        sed -i 's|http://archive.ubuntu.com/ubuntu|https://mirrors.tuna.tsinghua.edu.cn/ubuntu|g' /etc/apt/sources.list.d/ubuntu.sources; \
        sed -i 's|http://security.ubuntu.com/ubuntu|https://mirrors.tuna.tsinghua.edu.cn/ubuntu|g' /etc/apt/sources.list.d/ubuntu.sources; \
    fi; \
    rm -f /etc/apt/apt.conf.d/docker-clean && \
    echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' > /etc/apt/apt.conf.d/keep-cache && \
    chmod 1777 /tmp && \
    apt update && \
    apt install -y build-essential && \
    apt install -y libglib2.0-0 libglx-mesa0 libgl1 && \
    apt install -y pkg-config libicu-dev libgdiplus && \
    apt install -y default-jdk && \
    apt install -y libatk-bridge2.0-0 && \
    apt install -y libpython3-dev libgtk-4-1 libnss3 xdg-utils libgbm-dev && \
    apt install -y libjemalloc-dev && \
    apt install -y gnupg unzip curl wget git vim less && \
    apt install -y ghostscript && \
    apt install -y pandoc && \
    apt install -y texlive && \
    apt install -y fonts-freefont-ttf fonts-noto-cjk && \
    apt install -y postgresql-client

# Download resource from GitHub to /usr/share/infinity
RUN mkdir -p /usr/share/infinity/resource && \
    if [ "$NEED_MIRROR" == "1" ]; then \
        git clone --depth 1 --single-branch https://gitee.com/infiniflow/resource /tmp/resource; \
    else \
        git clone --depth 1 --single-branch https://github.com/infiniflow/resource.git /tmp/resource; \
    fi && \
    cp -r /tmp/resource/* /usr/share/infinity/resource && \
    rm -rf /tmp/resource

ARG NGINX_VERSION=1.29.5-1~noble
RUN --mount=type=cache,id=ragflow_apt,target=/var/cache/apt,sharing=locked \
    mkdir -p /etc/apt/keyrings && \
    curl --retry 5 --retry-delay 2 --retry-all-errors -fsSL https://nginx.org/keys/nginx_signing.key | gpg --dearmor -o /etc/apt/keyrings/nginx-archive-keyring.gpg && \
    echo "deb [signed-by=/etc/apt/keyrings/nginx-archive-keyring.gpg] https://nginx.org/packages/mainline/ubuntu/ noble nginx" > /etc/apt/sources.list.d/nginx.list && \
    apt -o Acquire::Retries=5 update && \
    apt -o Acquire::Retries=5 install -y nginx=${NGINX_VERSION} && \
    apt-mark hold nginx

# Install uv
RUN --mount=type=bind,from=infiniflow/ragflow_deps:latest,source=/,target=/deps \
    if [ "$NEED_MIRROR" == "1" ]; then \
        mkdir -p /etc/uv && \
        echo 'python-install-mirror = "https://registry.npmmirror.com/-/binary/python-build-standalone/"' > /etc/uv/uv.toml && \
        echo '[[index]]' >> /etc/uv/uv.toml && \
        echo 'url = "https://mirrors.aliyun.com/pypi/simple"' >> /etc/uv/uv.toml && \
        echo 'default = true' >> /etc/uv/uv.toml; \
    fi; \
    arch="$(uname -m)"; \
    if [ "$arch" = "x86_64" ]; then uv_arch="x86_64"; else uv_arch="aarch64"; fi; \
    tar xzf "/deps/uv-${uv_arch}-unknown-linux-gnu.tar.gz" \
    && cp "uv-${uv_arch}-unknown-linux-gnu/"* /usr/local/bin/ \
    && rm -rf "uv-${uv_arch}-unknown-linux-gnu" \
    && uv python install 3.12

ENV PYTHONDONTWRITEBYTECODE=1 DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1 \
    UV_HTTP_TIMEOUT=200 \
    UV_HTTP_RETRIES=3

# nodejs 12.22 on Ubuntu 22.04 is too old
RUN --mount=type=cache,id=ragflow_apt,target=/var/cache/apt,sharing=locked \
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash - && \
    apt purge -y nodejs npm && \
    apt autoremove -y && \
    apt update && \
    apt install -y nodejs

# Add msssql ODBC driver
# macOS ARM64 environment, install msodbcsql18.
# general x86_64 environment, install msodbcsql17.
RUN --mount=type=cache,id=ragflow_apt,target=/var/cache/apt,sharing=locked \
    curl https://packages.microsoft.com/keys/microsoft.asc | apt-key add - && \
    curl https://packages.microsoft.com/config/ubuntu/22.04/prod.list > /etc/apt/sources.list.d/mssql-release.list && \
    apt update && \
    arch="$(uname -m)"; \
    if [ "$arch" = "arm64" ] || [ "$arch" = "aarch64" ]; then \
        # ARM64 (macOS/Apple Silicon or Linux aarch64) \
        ACCEPT_EULA=Y apt install -y unixodbc-dev msodbcsql18; \
    else \
        # x86_64 or others \
        ACCEPT_EULA=Y apt install -y unixodbc-dev msodbcsql17; \
    fi || \
    { echo "Failed to install ODBC driver"; exit 1; }



# Add dependencies of selenium
RUN --mount=type=bind,from=infiniflow/ragflow_deps:latest,source=/chrome-linux64-121-0-6167-85,target=/chrome-linux64.zip \
    unzip /chrome-linux64.zip && \
    mv chrome-linux64 /opt/chrome && \
    ln -s /opt/chrome/chrome /usr/local/bin/
RUN --mount=type=bind,from=infiniflow/ragflow_deps:latest,source=/chromedriver-linux64-121-0-6167-85,target=/chromedriver-linux64.zip \
    unzip -j /chromedriver-linux64.zip chromedriver-linux64/chromedriver && \
    mv chromedriver /usr/local/bin/ && \
    rm -f /usr/bin/google-chrome

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

# install dependencies from uv.lock file
COPY pyproject.toml uv.lock ./

# https://github.com/astral-sh/uv/issues/10462
# uv records index url into uv.lock but doesn't failover among multiple indexes
RUN --mount=type=cache,id=ragflow_uv,target=/root/.cache/uv,sharing=locked \
    if [ "$NEED_MIRROR" == "1" ]; then \
        sed -i 's|pypi.org|mirrors.aliyun.com/pypi|g' uv.lock; \
    else \
        sed -i 's|mirrors.aliyun.com/pypi|pypi.org|g' uv.lock; \
    fi; \
    uv sync --python 3.12 --frozen && \
    # Ensure pip is available in the venv for runtime package installation (fixes #12651)
    .venv/bin/python3 -m ensurepip --upgrade

# Pre-install docling at build time (runtime install impossible in airgapped environments)
ARG DOCLING_VERSION=2.71.0
RUN --mount=type=cache,id=ragflow_uv,target=/root/.cache/uv,sharing=locked \
    uv pip install --no-cache-dir "docling==${DOCLING_VERSION}"

COPY web web
COPY docs docs
RUN --mount=type=cache,id=ragflow_npm,target=/root/.npm,sharing=locked \
    export NODE_OPTIONS="--max-old-space-size=4096" && \
    cd web && npm install && npm run build

COPY .git /ragflow/.git

RUN version_info=$(git describe --tags --match=v* --first-parent --always); \
    version_info="$version_info"; \
    echo "RAGFlow version: $version_info"; \
    echo $version_info > /ragflow/VERSION

# production stage
FROM base AS production
USER root

WORKDIR /ragflow

# Copy Python environment and packages
ENV VIRTUAL_ENV=/ragflow/.venv
COPY --from=builder ${VIRTUAL_ENV} ${VIRTUAL_ENV}
ENV PATH="${VIRTUAL_ENV}/bin:/usr/local/bin:/usr/bin:/bin"

ENV PYTHONPATH=/ragflow/

COPY web web
COPY admin admin
COPY api api
COPY conf conf
COPY deepdoc deepdoc
COPY rag rag
COPY agent agent
COPY pyproject.toml uv.lock ./
COPY mcp mcp
COPY common common
COPY memory memory
COPY bin bin

COPY docker/service_conf.yaml.template ./conf/service_conf.yaml.template
COPY docker/entrypoint.sh ./
RUN chmod +x ./entrypoint*.sh

# Copy nginx configuration for frontend serving
COPY docker/nginx/ragflow.conf.golang docker/nginx/ragflow.conf.python docker/nginx/ragflow.conf.hybrid docker/nginx/nginx.conf docker/nginx/proxy.conf /etc/nginx/
RUN mv /etc/nginx/ragflow.conf.golang /etc/nginx/conf.d/ragflow.conf.golang && \
    mv /etc/nginx/ragflow.conf.python /etc/nginx/conf.d/ragflow.conf.python && \
    mv /etc/nginx/ragflow.conf.hybrid /etc/nginx/conf.d/ragflow.conf.hybrid && \
    rm -f /etc/nginx/sites-enabled/default

# Copy compiled web pages
COPY --from=builder /ragflow/web/dist /ragflow/web/dist

COPY --from=builder /ragflow/VERSION /ragflow/VERSION

# =============================================================================
# OPENSHIFT RESTRICTED-V2 COMPATIBILITY
# =============================================================================
 
RUN rm -rf /root/nltk_data /root/.ragflow 2>/dev/null || true
 
# 2. Configure nginx for non-root:
RUN sed -i -E 's/listen\s+80(\s|;)/listen 8080\1/g' /etc/nginx/conf.d/*.conf* /etc/nginx/conf.d/* 2>/dev/null || true && \
    sed -i 's|pid\s\+/var/run/nginx.pid|pid /tmp/nginx.pid|g' /etc/nginx/nginx.conf && \
    grep -q '^\s*pid ' /etc/nginx/nginx.conf || sed -i '1i pid /tmp/nginx.pid;' /etc/nginx/nginx.conf && \
    echo 'client_body_temp_path /tmp/nginx_client_body;' > /etc/nginx/conf.d/00-temp-paths.conf && \
    echo 'proxy_temp_path /tmp/nginx_proxy;' >> /etc/nginx/conf.d/00-temp-paths.conf && \
    echo 'fastcgi_temp_path /tmp/nginx_fastcgi;' >> /etc/nginx/conf.d/00-temp-paths.conf && \
    echo 'uwsgi_temp_path /tmp/nginx_uwsgi;' >> /etc/nginx/conf.d/00-temp-paths.conf && \
    echo 'scgi_temp_path /tmp/nginx_scgi;' >> /etc/nginx/conf.d/00-temp-paths.conf
 
# 3. Set HOME to /tmp 
ENV HOME=/tmp
 
# 4. Set group ownership to GID 0 and mirror permissions 
RUN chgrp -R 0 /ragflow \
                /opt/nltk_data \
                /opt/ragflow_home \
                /opt/chrome \
                /var/log/nginx \
                /var/cache/nginx \
                /var/run \
                /etc/nginx \
    && chmod -R g=u /ragflow \
                    /opt/nltk_data \
                    /opt/ragflow_home \
                    /opt/chrome \
                    /var/log/nginx \
                    /var/cache/nginx \
                    /var/run \
                    /etc/nginx \
    && chmod -R g=u /tmp
 
# 5. Set non-root user
USER 1000

ENTRYPOINT ["./entrypoint.sh"]
