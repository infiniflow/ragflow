#!/bin/bash

# replace env variables in the service_conf.yaml file
rm -rf /ragflow/conf/service_conf.yaml
while IFS= read -r line || [[ -n "$line" ]]; do
    # Use eval to interpret the variable with default values
    eval "echo \"$line\"" >> /ragflow/conf/service_conf.yaml
done < /ragflow/conf/service_conf.yaml.template

# update RAGFlow version
# Get the latest tag
last_tag=$(git describe --tags --abbrev=0)
# Get the number of commits from the last tag
commit_count=$(git rev-list --count "$last_tag..HEAD")
# Get the short commit id
last_commit=$(git rev-parse --short HEAD)

version_info=""
if [ "$commit_count" -eq 0 ]; then
    version_info=$last_tag
else
    printf -v version_info "%s(%s~%d)" "$last_commit" "$last_tag" $commit_count
fi
# Replace the version in the versions.py file
sed -i "s/\"dev\"/\"$version_info\"/" /api/versions.py

# unset http proxy which maybe set by docker daemon
export http_proxy=""; export https_proxy=""; export no_proxy=""; export HTTP_PROXY=""; export HTTPS_PROXY=""; export NO_PROXY=""

/usr/sbin/nginx

export LD_LIBRARY_PATH=/usr/lib/x86_64-linux-gnu/

PY=python3
if [[ -z "$WS" || $WS -lt 1 ]]; then
  WS=1
fi

function task_exe(){
    while [ 1 -eq 1 ];do
      $PY rag/svr/task_executor.py $1;
    done
}

for ((i=0;i<WS;i++))
do
  task_exe  $i &
done

while [ 1 -eq 1 ];do
    $PY api/ragflow_server.py
done

wait;
