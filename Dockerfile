FROM infiniflow/ragflow-base:v1.0

WORKDIR /ragflow

COPY . ./
RUN cd ./web && npm i && npm build

ENV PYTHONPATH=/ragflow
ENV HF_ENDPOINT=https://hf-mirror.com

COPY docker/entrypoint.sh ./
RUN chmod +x ./entrypoint.sh

ENTRYPOINT ["/bin/bash", "./entrypoint.sh"]