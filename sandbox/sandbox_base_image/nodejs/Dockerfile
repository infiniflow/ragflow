FROM node:24-bookworm-slim

RUN npm config set registry https://registry.npmmirror.com

# RUN grep -rl 'deb.debian.org' /etc/apt/ | xargs sed -i 's|http[s]*://deb.debian.org|https://mirrors.ustc.edu.cn|g' && \
#     apt-get update && \
#     apt-get install -y curl gcc make


WORKDIR /app

COPY package.json package-lock.json .

RUN npm install

CMD ["sleep", "infinity"]

