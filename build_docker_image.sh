#!/bin/bash

print_help() {
    echo "Usage: $0 <option>"
    echo "    full, build full image"
    echo "    slim, build slim image"
    exit 1
}

if [ "$#" -ne 1 ]; then
    print_help
fi

docker_version="full"
if [ "$1" == "full" ]; then
    docker_version="full"
elif [ "$1" == "slim" ]; then
    docker_version="slim"
else
    print_help
fi

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
sed -i "s/\"dev\"/\"$version_info\"/" api/versions.py

if [ "$docker_version" == "full" ]; then
    docker build -f Dockerfile -t infiniflow/ragflow:dev .
else
    docker build -f Dockerfile.slim -t infiniflow/ragflow:dev-slim .
fi

git restore api/versions.py