FROM infiniflow/ragflow-base:v2.0
USER  root

WORKDIR /ragflow

ADD ./web ./web
RUN cd ./web && npm i --force && npm run build

ADD ./api ./api
ADD ./conf ./conf
ADD ./deepdoc ./deepdoc
ADD ./rag ./rag
ADD ./agent ./agent
ADD ./graphrag ./graphrag

ENV PYTHONPATH=/ragflow/
ENV HF_ENDPOINT=https://hf-mirror.com

ADD docker/entrypoint.sh ./entrypoint.sh
ADD docker/.env ./
RUN chmod +x ./entrypoint.sh

ENTRYPOINT ["./entrypoint.sh"]