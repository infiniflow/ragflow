FROM ubuntu:22.04 as base

RUN apt-get update 

ENV TZ="Asia/Taipei"
RUN apt-get install -yq  \
    build-essential \
    curl \
    libncursesw5-dev \
    libssl-dev \
    libsqlite3-dev \
    libgdbm-dev \
    libc6-dev \
    libbz2-dev \
    software-properties-common \
    python3.11 python3.11-dev python3-pip

RUN apt-get install -yq  git
RUN pip3 config set global.index-url https://mirror.baidu.com/pypi/simple
RUN pip3 config set global.trusted-host mirror.baidu.com
RUN pip3 install --upgrade pip
RUN pip3 install torch==2.0.1 
RUN pip3 install torch-model-archiver==0.8.2
RUN pip3 install torchvision==0.15.2
COPY requirements.txt .

WORKDIR /docgpt
ENV PYTHONPATH=/docgpt/

