FROM python:3.11-slim-bookworm

ARG NEED_MIRROR=1


COPY --from=ghcr.io/astral-sh/uv:0.7.5 /uv /uvx /bin/
ENV UV_INDEX_URL = if [ "$NEED_MIRROR" = 1 ]; then echo "https://pypi.tuna.tsinghua.edu.cn/simple"; else echo "https://pypi.org/simple"; fi
ENV MPLBACKEND=Agg
ENV MPLCONFIGDIR=/tmp/matplotlib
ENV MATPLOTLIBRC=/usr/local/etc/matplotlibrc

COPY requirements.txt .
COPY matplotlibrc /usr/local/etc/matplotlibrc

RUN if [ "$NEED_MIRROR" = 1 ]; then \
        grep -rl 'deb.debian.org' /etc/apt/ | xargs sed -i 's|http[s]*://deb.debian.org|https://mirrors.tuna.tsinghua.edu.cn|g'; \
    fi; \
    apt-get update && \
    apt-get install -y curl gcc && \
    mkdir -p /tmp/matplotlib && \
    uv pip install --system -r requirements.txt

WORKDIR /workspace

CMD ["sleep", "infinity"]
