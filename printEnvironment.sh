#!/bin/bash

# 函数用于获取发行版信息
get_distro_info() {
    local distro_id=$(lsb_release -i -s 2>/dev/null)
    local distro_version=$(lsb_release -r -s 2>/dev/null)
    local kernel_version=$(uname -r)

    # 如果lsb_release不可用，尝试解析/etc/*-release文件
    if [ -z "$distro_id" ] || [ -z "$distro_version" ]; then
        distro_id=$(grep '^ID=' /etc/*-release | cut -d= -f2 | tr -d '"')
        distro_version=$(grep '^VERSION_ID=' /etc/*-release | cut -d= -f2 | tr -d '"')
    fi

    echo "$distro_id $distro_version (内核版本: $kernel_version)"
}

# 获取当前目录的Git仓库名称
git_repo_name=''
if git rev-parse --is-inside-work-tree > /dev/null 2>&1; then
    git_repo_name=$(basename "$(git rev-parse --show-toplevel)")
    if [ $? -ne 0 ]; then
        git_repo_name="(无法获取仓库名称)"
    fi
else
    git_repo_name="不在Git仓库中"
fi

# 获取CPU型号
cpu_model=$(uname -m)

# 获取内存容量
memory_size=$(free -h | grep Mem | awk '{print $2}')

# 获取Docker版本
docker_version=''
if command -v docker &> /dev/null; then
    docker_version=$(docker --version | cut -d ' ' -f3)
else
    docker_version="Docker未安装"
fi

# 获取Python版本
python_version=''
if command -v python &> /dev/null; then
    python_version=$(python --version | cut -d ' ' -f2)
else
    python_version="Python未安装"
fi

# 打印所有信息
echo "当前仓库的名称是：$git_repo_name"
echo "操作系统: $(get_distro_info)"
echo "CPU类型：$cpu_model"
echo "内存容量：$memory_size"
echo "Docker版本：$docker_version"
echo "Python版本：$python_version"
