#!/bin/bash

# The function is used to obtain distribution information
get_distro_info() {
    local distro_id=$(lsb_release -i -s 2>/dev/null)
    local distro_version=$(lsb_release -r -s 2>/dev/null)
    local kernel_version=$(uname -r)

    # If lsb_release is not available, try parsing the /etc/*-release file
    if [ -z "$distro_id" ] || [ -z "$distro_version" ]; then
        distro_id=$(grep '^ID=' /etc/*-release | cut -d= -f2 | tr -d '"')
        distro_version=$(grep '^VERSION_ID=' /etc/*-release | cut -d= -f2 | tr -d '"')
    fi

    echo "$distro_id $distro_version (Kernel version: $kernel_version)"
}

# get Git repository name
git_repo_name=''
if git_toplevel=$(git rev-parse --show-toplevel 2>/dev/null); then
    git_repo_name=$(basename "$git_toplevel")
elif git_dir=$(git rev-parse --absolute-git-dir 2>/dev/null); then
    # Inside .git, or in a bare repo: there is no work tree to name, but this is
    # still a repository. Ask git whether it is bare rather than reading the
    # path, since a bare repo may itself be called ".git". A bare repository
    # names itself; otherwise the git directory's parent is the repository.
    if [ "$(git rev-parse --is-bare-repository 2>/dev/null)" = "true" ]; then
        git_repo_name=$(basename "$git_dir")
    else
        # A linked worktree's git dir lives under .git/worktrees, so the main
        # repository is found through the common dir. That is reported relative
        # to the git dir (plain "." inside .git), hence resolving it here.
        git_common=$(cd "$git_dir" && cd "$(git rev-parse --git-common-dir)" && pwd)
        git_repo_name=$(basename "$(dirname "$git_common")")
    fi
else
    git_repo_name="It is NOT a Git repo"
fi

# get CPU type
cpu_model=$(uname -m)

# get memory size
memory_size=$(free -h | grep Mem | awk '{print $2}')

# get docker version
docker_version=''
if command -v docker &> /dev/null; then
    docker_version=$(docker --version | cut -d ' ' -f3)
else
    docker_version="Docker not installed"
fi

# get python version
python_version=$(python3 --version 2>&1 || python --version 2>&1 || echo "Python not installed")

# Print all information
echo "Current Repository: $git_repo_name"

# get Commit ID
git_version=$(git log -1 --pretty=format:'%h' 2>/dev/null)

if [ -z "$git_version" ]; then
    echo "Commit Id: The current directory is not a Git repository, or the Git command is not installed."
else
    echo "Commit Id: $git_version"
fi

echo "Operating system: $(get_distro_info)"
echo "CPU Type: $cpu_model"
echo "Memory: $memory_size"
echo "Docker Version: $docker_version"
echo "Python Version: $python_version"
