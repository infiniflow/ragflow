#!/bin/bash

# 仓库配置
ORIGINAL_REPO="https://github.com/infiniflow/ragflow.git"  # 源仓库URL
FORK_REPO="https://github.com/hippoley/ragflow.git"        # 你fork的仓库URL

# 设置颜色输出
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# 打印带颜色的消息
function print_message() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

function print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

function print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查当前目录是否是Git仓库
if [ ! -d ".git" ]; then
    print_error "当前目录不是Git仓库！"
    
    # 询问是否要克隆仓库
    read -p "是否要克隆fork的仓库 ($FORK_REPO)? (y/n): " clone_repo
    if [[ $clone_repo == "y" || $clone_repo == "Y" ]]; then
        git clone $FORK_REPO
        cd $(basename $FORK_REPO .git)
        print_message "已克隆并进入仓库目录"
    else
        print_error "请先进入Git仓库目录再运行此脚本"
        exit 1
    fi
fi

# 检查上游仓库是否已设置
if ! git remote | grep -q "upstream"; then
    print_message "未找到upstream远程仓库，正在添加..."
    git remote add upstream "$ORIGINAL_REPO"
    print_message "已添加upstream远程仓库: $ORIGINAL_REPO"
else
    # 如果上游仓库已经设置，验证它是否正确
    current_upstream=$(git remote get-url upstream 2>/dev/null)
    if [ "$current_upstream" != "$ORIGINAL_REPO" ]; then
        print_warning "当前upstream设置 ($current_upstream) 与预期不符"
        read -p "是否更新upstream地址为 $ORIGINAL_REPO? (y/n): " update_upstream
        if [[ $update_upstream == "y" || $update_upstream == "Y" ]]; then
            git remote set-url upstream "$ORIGINAL_REPO"
            print_message "已更新upstream远程仓库地址"
        fi
    else
        print_message "验证upstream远程仓库已正确设置"
    fi
fi

# 验证origin远程仓库是否正确
current_origin=$(git remote get-url origin 2>/dev/null)
if [ "$current_origin" != "$FORK_REPO" ]; then
    print_warning "当前origin设置 ($current_origin) 与预期不符"
    read -p "是否更新origin地址为 $FORK_REPO? (y/n): " update_origin
    if [[ $update_origin == "y" || $update_origin == "Y" ]]; then
        git remote set-url origin "$FORK_REPO"
        print_message "已更新origin远程仓库地址"
    fi
else
    print_message "验证origin远程仓库已正确设置"
fi

# 保存当前分支名称
current_branch=$(git branch --show-current)
print_message "当前分支: $current_branch"

# 1. 获取最新的上游仓库数据
print_message "从upstream获取最新更新..."
if ! git fetch upstream; then
    print_error "从upstream获取更新失败！请检查网络连接和仓库权限。"
    exit 1
fi

# 2. 切换到main分支并同步
print_message "切换到main分支..."
if ! git checkout main; then
    print_warning "切换到main分支失败，尝试检查是否是master分支..."
    if ! git checkout master; then
        print_error "无法找到main或master分支！请确认主分支名称。"
        exit 1
    else
        main_branch="master"
    fi
else
    main_branch="main"
fi

print_message "合并upstream/$main_branch到本地$main_branch分支..."
if ! git merge upstream/$main_branch; then
    print_error "合并失败！可能存在冲突需要手动解决。"
    print_message "请解决冲突后再运行此脚本。"
    exit 1
fi

# 3. 检查dev-persistence分支并切换
print_message "检查dev-persistence分支..."
git_dev_exists=$(git branch --list dev-persistence)
if [ -z "$git_dev_exists" ]; then
    print_warning "dev-persistence分支不存在！"
    read -p "是否要从$main_branch创建dev-persistence分支? (y/n): " create_branch
    if [[ $create_branch == "y" || $create_branch == "Y" ]]; then
        git checkout -b dev-persistence
        print_message "已创建新的dev-persistence分支"
    else
        git checkout $current_branch
        print_error "操作取消，已切换回 $current_branch 分支"
        exit 1
    fi
else
    # 如果分支已存在，切换到它
    print_message "切换到dev-persistence分支..."
    git checkout dev-persistence
    
    # 检查是否有未提交的更改
    if ! git diff --quiet; then
        print_warning "dev-persistence分支有未提交的更改"
        read -p "是否要提交这些更改? (y/n): " commit_changes
        if [[ $commit_changes == "y" || $commit_changes == "Y" ]]; then
            read -p "输入提交信息: " commit_msg
            git add .
            git commit -m "$commit_msg"
            print_message "已提交更改"
        else
            print_warning "继续操作可能会导致冲突..."
        fi
    fi
fi

# 4. 合并main分支到dev-persistence（保留dev-persistence的修改）
print_message "合并$main_branch分支到dev-persistence分支..."
print_message "这个操作会保留dev-persistence分支上的所有自定义修改，同时将$main_branch的新更新添加进来"
if ! git merge $main_branch; then
    print_error "合并过程中发生冲突！"
    print_message "请手动解决冲突:"
    print_message "1. 编辑冲突文件，保留你想要的内容"
    print_message "2. 使用 'git add <冲突文件>' 标记为已解决"
    print_message "3. 使用 'git merge --continue' 完成合并"
    print_message "或者，如果你想取消此次合并:"
    print_message "   使用 'git merge --abort'"
    exit 1
fi

print_message "成功将$main_branch分支的更新合并到dev-persistence，同时保留了dev-persistence上的修改"

# 5. 询问是否推送到远程
read -p "是否将更新推送到远程fork仓库? (y/n): " push_to_remote
if [[ $push_to_remote == "y" || $push_to_remote == "Y" ]]; then
    print_message "推送$main_branch分支到origin..."
    git push origin $main_branch
    
    print_message "推送dev-persistence分支到origin..."
    git push origin dev-persistence
    
    print_message "同步完成并已推送到远程仓库！"
else
    print_message "同步完成！(未推送到远程仓库)"
fi

# 是否切回原来的分支
if [[ $current_branch != $main_branch && $current_branch != "dev-persistence" ]]; then
    read -p "是否切回原来的分支 $current_branch? (y/n): " switch_back
    if [[ $switch_back == "y" || $switch_back == "Y" ]]; then
        git checkout $current_branch
        print_message "已切换回 $current_branch 分支"
    fi
fi

print_message "同步操作全部完成！"