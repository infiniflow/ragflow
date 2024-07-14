FROM infiniflow/ragflow-base:v2.0
USER  root

WORKDIR /ragflow

RUN npm install -g npm@latest
RUN pip install --no-cache-dir selenium
RUN pip install webdriver-manager
RUN pip install roman_numbers
RUN pip install word2number
RUN pip install cn2an
RUN pip install markdown

ADD ./web ./web
RUN cd ./web && npm i --force && npm run build

ADD ./api ./api
ADD ./conf ./conf
ADD ./deepdoc ./deepdoc
ADD ./rag ./rag
ADD ./graph ./graph

ENV PYTHONPATH=/ragflow/
ENV HF_ENDPOINT=https://hf-mirror.com

ADD docker/entrypoint.sh ./entrypoint.sh
ADD docker/.env ./
RUN chmod +x ./entrypoint.sh

ENTRYPOINT ["./entrypoint.sh"]