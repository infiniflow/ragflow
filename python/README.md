
```shell

docker pull postgres

LOCAL_POSTGRES_DATA=./postgres-data

docker run
    --name docass-postgres
    -p 5455:5432
    -v $LOCAL_POSTGRES_DATA:/var/lib/postgresql/data
    -e POSTGRES_USER=root
    -e POSTGRES_PASSWORD=infiniflow_docass
    -e POSTGRES_DB=docass
    -d
    postgres

docker network create elastic
docker pull elasticsearch:8.11.3; 
docker pull docker.elastic.co/kibana/kibana:8.11.3

```
