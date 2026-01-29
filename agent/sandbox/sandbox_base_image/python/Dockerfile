FROM python:3.11-slim-bookworm

COPY --from=ghcr.io/astral-sh/uv:0.7.5 /uv /uvx /bin/
ENV UV_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple

COPY requirements.txt .

RUN grep -rl 'deb.debian.org' /etc/apt/ | xargs sed -i 's|http[s]*://deb.debian.org|https://mirrors.tuna.tsinghua.edu.cn|g' && \
    apt-get update && \
    apt-get install -y curl gcc && \
    uv pip install --system -r requirements.txt

WORKDIR /workspace

CMD ["sleep", "infinity"]
