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
