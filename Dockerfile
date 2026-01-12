# base stage
FROM ubuntu:24.04 AS base
USER root
SHELL ["/bin/bash", "-c"]

ARG NEED_MIRROR=0

WORKDIR /ragflow

# Set uv python install directory
ENV UV_PYTHON_INSTALL_DIR=/usr/local/share/uv/python

# Copy models downloaded via download_deps.py
RUN mkdir -p /ragflow/rag/res/deepdoc
RUN --mount=type=bind,from=infiniflow/ragflow_deps:latest,source=/huggingface.co,target=/huggingface.co \
    tar --exclude='.*' -cf - \
        /huggingface.co/InfiniFlow/text_concat_xgb_v1.0 \
        /huggingface.co/InfiniFlow/deepdoc \
        | tar -xf - --strip-components=3 -C /ragflow/rag/res/deepdoc

# https://github.com/chrismattmann/tika-python
# This is the only way to run python-tika without internet access. Without this set, the default is to check the tika version and pull latest every time from Apache.
RUN --mount=type=bind,from=infiniflow/ragflow_deps:latest,source=/,target=/deps \
    cp -r /deps/nltk_data /usr/share/nltk_data && \
    cp /deps/tika-server-standard-3.2.3.jar /deps/tika-server-standard-3.2.3.jar.md5 /ragflow/ && \
    cp /deps/cl100k_base.tiktoken /ragflow/9b5ad71b2ce5302211f9c61530b329a4922fc6a4

ENV NLTK_DATA=/usr/share/nltk_data
ENV TIKA_SERVER_JAR="file:///ragflow/tika-server-standard-3.2.3.jar"
ENV DEBIAN_FRONTEND=noninteractive

# Setup apt
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
    apt install -y libglib2.0-0 libglx-mesa0 libgl1 && \
    apt install -y pkg-config libicu-dev libgdiplus && \
    apt install -y default-jdk && \
    apt install -y libatk-bridge2.0-0 && \
    apt install -y libpython3-dev libgtk-4-1 libnss3 xdg-utils libgbm-dev && \
    apt install -y libjemalloc-dev && \
    apt install -y nginx unzip curl wget git vim less sudo && \
    apt install -y ghostscript && \
    apt install -y pandoc && \
    apt install -y texlive && \
    apt install -y fonts-freefont-ttf fonts-noto-cjk

# Install uv
RUN --mount=type=bind,from=infiniflow/ragflow_deps:latest,source=/,target=/deps \
    if [ "$NEED_MIRROR" == "1" ]; then \
        mkdir -p /etc/uv && \
        echo 'python-install-mirror = "https://registry.npmmirror.com/-/binary/python-build-standalone/"' > /etc/uv/uv.toml && \
        echo '[[index]]' >> /etc/uv/uv.toml && \
        echo 'url = "https://pypi.tuna.tsinghua.edu.cn/simple"' >> /etc/uv/uv.toml && \
        echo 'default = true' >> /etc/uv/uv.toml; \
    fi; \
    tar xzf /deps/uv-x86_64-unknown-linux-gnu.tar.gz \
    && cp uv-x86_64-unknown-linux-gnu/* /usr/local/bin/ \
    && rm -rf uv-x86_64-unknown-linux-gnu \
    && uv python install 3.12

ENV PYTHONDONTWRITEBYTECODE=1 DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1
ENV PATH=/root/.local/bin:$PATH

# nodejs 20.x
RUN --mount=type=cache,id=ragflow_apt,target=/var/cache/apt,sharing=locked \
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash - && \
    apt purge -y nodejs npm cargo && \
    apt autoremove -y && \
    apt update && \
    apt install -y nodejs

# Add msssql ODBC driver
RUN --mount=type=cache,id=ragflow_apt,target=/var/cache/apt,sharing=locked \
    curl https://packages.microsoft.com/keys/microsoft.asc | apt-key add - && \
    curl https://packages.microsoft.com/config/ubuntu/22.04/prod.list > /etc/apt/sources.list.d/mssql-release.list && \
    apt update && \
    arch="$(uname -m)"; \
    if [ "$arch" = "arm64" ] || [ "$arch" = "aarch64" ]; then \
        ACCEPT_EULA=Y apt install -y unixodbc-dev msodbcsql18; \
    else \
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

# aspose-slides
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
COPY pyproject.toml uv.lock ./
RUN --mount=type=cache,id=ragflow_uv,target=/root/.cache/uv,sharing=locked \
    if [ "$NEED_MIRROR" == "1" ]; then \
        sed -i 's|pypi.org|pypi.tuna.tsinghua.edu.cn|g' uv.lock; \
    else \
        sed -i 's|pypi.tuna.tsinghua.edu.cn|pypi.org|g' uv.lock; \
    fi; \
    uv sync --python 3.12 --frozen

COPY web web
COPY docs docs
RUN --mount=type=cache,id=ragflow_npm,target=/root/.npm,sharing=locked \
    cd web && npm install && npm run build

COPY .git /ragflow/.git
RUN version_info=$(git describe --tags --match=v* --first-parent --always); \
    echo $version_info > /ragflow/VERSION

# production stage
FROM base AS production
RUN useradd -m -u 1001 ragflow
WORKDIR /ragflow

# Copy Python environment and packages
ENV VIRTUAL_ENV=/ragflow/.venv
COPY --from=builder --chown=ragflow:ragflow ${VIRTUAL_ENV} ${VIRTUAL_ENV}
ENV PATH="${VIRTUAL_ENV}/bin:${PATH}"
ENV PYTHONPATH=/ragflow/

COPY --chown=ragflow:ragflow web web
COPY --chown=ragflow:ragflow admin admin
COPY --chown=ragflow:ragflow api api
COPY --chown=ragflow:ragflow conf conf
COPY --chown=ragflow:ragflow deepdoc deepdoc
COPY --chown=ragflow:ragflow rag rag
COPY --chown=ragflow:ragflow agent agent
COPY --chown=ragflow:ragflow graphrag graphrag
COPY --chown=ragflow:ragflow agentic_reasoning agentic_reasoning
COPY --chown=ragflow:ragflow pyproject.toml uv.lock ./
COPY --chown=ragflow:ragflow mcp mcp
COPY --chown=ragflow:ragflow plugin plugin
COPY --chown=ragflow:ragflow common common
COPY --chown=ragflow:ragflow memory memory

COPY --chown=ragflow:ragflow docker/service_conf.yaml.template ./conf/service_conf.yaml.template
COPY --chown=ragflow:ragflow docker/entrypoint.sh ./
RUN chmod +x ./entrypoint*.sh

# Copy compiled web pages
COPY --from=builder --chown=ragflow:ragflow /ragflow/web/dist /ragflow/web/dist
COPY --from=builder --chown=ragflow:ragflow /ragflow/VERSION /ragflow/VERSION

RUN touch /tmp/nginx.pid && \
    chown -R ragflow:ragflow /var/lib/nginx /var/log/nginx /etc/nginx /tmp/nginx.pid

RUN chown -R ragflow:ragflow /ragflow
USER root

ENTRYPOINT ["./entrypoint.sh"]
