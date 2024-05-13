FROM swr.cn-north-4.myhuaweicloud.com/infiniflow/ragflow-base:v1.0
USER  root

WORKDIR /ragflow

ENV PYTHONPATH=/ragflow/
ENV HF_ENDPOINT=https://hf-mirror.com

ENV CONDA_DEFAULT_ENV py11
ENV CONDA_PREFIX /root/miniconda3/envs/py11
ENV PATH /root/miniconda3/bin:$PATH
ENV PATH $CONDA_PREFIX/bin:$PATH

ADD ./requirements.txt ./requirements.txt
RUN conda run -n py11 pip install -i https://mirrors.aliyun.com/pypi/simple/ -r requirements.txt
RUN conda run -n py11 pip install -i https://mirrors.aliyun.com/pypi/simple/ pocketbase --no-deps

ADD ./web ./web
RUN cd ./web && npm i --force && npm run build

ADD ./api ./api
ADD ./conf ./conf
ADD ./deepdoc ./deepdoc
ADD ./rag ./rag

ADD docker/entrypoint.sh ./entrypoint.sh
RUN chmod +x ./entrypoint.sh

ENTRYPOINT ["./entrypoint.sh"]