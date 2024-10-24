
Now, ```ragflow``` will be split into three components, namely ```ragflow-web```, ```ragflow-api```, ```ragflow-worker```.

- Only CPU resources are needed for ```ragflow-web```, ```ragflow-api```.
- If you want faster search speed, you can use GPU to deploy ```ragflow-worker```.
- Also, you can separately expand multiple instances for ```ragflow-worker```.

## deploy from source-code
```shell
docker-compose --env-file=.env -f docker-compose-dmrj-source.yml up -d
```

## scale worker replicas
```shell
docker-compose --env-file=.env -f docker-compose-source.yml up -d --scale ragflow-worker=2
```


