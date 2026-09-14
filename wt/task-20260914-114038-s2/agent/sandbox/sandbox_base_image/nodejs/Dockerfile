FROM node:24.13-bookworm-slim

ARG NEED_MIRROR=1

RUN if [ "$NEED_MIRROR" = 1 ]; then \
        npm config set registry https://registry.npmmirror.com; \
    else \
        npm config set registry https://registry.npmjs.org; \
    fi

# RUN grep -rl 'deb.debian.org' /etc/apt/ | xargs sed -i 's|http[s]*://deb.debian.org|https://mirrors.ustc.edu.cn|g' && \
#     apt-get update && \
#     apt-get install -y curl gcc make


WORKDIR /app

COPY package.json package-lock.json .

RUN npm install

CMD ["sleep", "infinity"]

