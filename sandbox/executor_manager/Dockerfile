FROM python:3.11-slim-bookworm

RUN grep -rl 'deb.debian.org' /etc/apt/ | xargs sed -i 's|http[s]*://deb.debian.org|https://mirrors.tuna.tsinghua.edu.cn|g' && \
    apt-get update && \
    apt-get install -y curl gcc && \
    rm -rf /var/lib/apt/lists/*

RUN curl -fsSL https://mirrors.aliyun.com/docker-ce/linux/static/stable/x86_64/docker-24.0.7.tgz -o docker.tgz && \
    tar -xzf docker.tgz && \
    mv docker/docker /usr/bin/docker && \
    rm -rf docker docker.tgz

COPY --from=ghcr.io/astral-sh/uv:0.7.5 /uv /uvx /bin/
ENV UV_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple


WORKDIR /app
COPY . .

RUN uv pip install --system -r requirements.txt

CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "9385"]

