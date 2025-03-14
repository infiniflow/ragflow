#!/bin/bash

echo "切换当前python脚本到虚拟解释器 .venv/bin/activate"
source .venv/bin/activate

# 检查并删除现有的 .pid 文件
if ls *.pid 1> /dev/null 2>&1; then
    echo "发现现有的 .pid 文件，正在删除..."
    rm -v *.pid
else
    echo "未发现现有的 .pid 文件。"
fi

# 打印启动命令
echo "启动命令：nohup bash launch_backend_service.sh > nohup.out 2>&1 &"
# 启动后台进程
nohup bash docker/launch_backend_service.sh > nohup.out 2>&1 &

# 获取 PID
PID=$!

# 生成以 PID 为文件名的空文件
touch "${PID}.pid"

# 打印提示信息
echo "launch_backend_service.sh 已启动，PID 为 ${PID}，空文件已生成：${PID}.pid"