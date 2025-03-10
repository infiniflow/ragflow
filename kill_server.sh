#!/bin/bash
echo "---------------------------------------------------------------------------"
echo "开始关闭主进程..."
# 检查是否存在 .pid 文件
if ls *.pid 1> /dev/null 2>&1; then
    # 获取 .pid 文件名
    PID_FILE=$(ls *.pid)

    # 从文件名中提取 PID
    PID=${PID_FILE%.pid}

    # 打印提示信息
    echo "正在停止进程 (PID: ${PID})..."

    # 使用 kill 停止进程
    kill "${PID}"

    # 等待几秒钟
    sleep 8

    # 检查进程是否已停止
    if ps -ef | grep launch_backend_service.sh | grep -v grep > /dev/null; then
        echo "进程停止失败，请手动检查。"
    else
        echo "程序已成功停止。"
        # 删除 .pid 文件
        rm -v "${PID_FILE}"
    fi
else
    echo "未找到 .pid 文件，程序可能未启动。"
fi
#
## -----------------------------停止在端口上监听的Flask进程-------------------------
#echo "---------------------------------------------------------------------------"
#echo "开始关闭Flask服务器进程..."
#
## 从 YAML 文件中读取 http_port 的值
#HTTP_PORT=$(yq -r '.ragflow.http_port' conf/service_conf.yaml 2>/dev/null)
#
## 检查是否成功读取端口号
#if [ -z "$HTTP_PORT" ]; then
#    echo "无法从 conf/service_conf.yaml 中读取 http_port 的值"
#    exit 1
#fi
#
## 查找运行在指定端口的进程
#PID=$(lsof -i :$HTTP_PORT -t 2>/dev/null)
#
#if [ -z "$PID" ]; then
#    echo "没有找到运行在 $HTTP_PORT 端口的进程"
#else
#    echo "找到运行在 $HTTP_PORT 端口的进程, 进程ID: $PID，正在关闭..."
#    # 先尝试正常终止进程
#    kill $PID 2>/dev/null
#    # 等待进程退出
#    sleep 2
#    # 检查进程是否仍然存在
#    if ps -p $PID > /dev/null 2>&1; then
#        echo "进程 PID: $PID 未正常关闭，尝试强制终止..."
#        kill -9 $PID 2>/dev/null
#        sleep 1
#        if ps -p $PID > /dev/null 2>&1; then
#            echo "进程 PID: $PID 强制终止失败，请手动检查"
#            exit 1
#        else
#            echo "进程 PID: $PID 已强制终止"
#        fi
#    else
#        echo "进程 PID: $PID 已正常关闭"
#    fi
#fi
#

# -----------------------------清理孤儿进程-------------------------
echo "---------------------------------------------------------------------------"
echo "开始未清理干净的孤儿进程脚本..."

# 查找并终止 task_executor.py 进程
echo "查找 rag/svr/task_executor.py 进程..."
TASK_PIDS=$(ps -ef | grep "python3.*rag/svr/task_executor.py" | grep -v grep | awk '{print $2}')

if [ -z "$TASK_PIDS" ]; then
    echo "未找到 task_executor.py 进程"
else
    echo "找到以下 task_executor.py 进程:"
    for PID in $TASK_PIDS; do
        CMDLINE=$(cat /proc/$PID/cmdline 2>/dev/null | tr '\0' ' ' || echo "无法读取")
        echo "PID: $PID - 命令: $CMDLINE"
    done

    echo "正在终止这些进程..."
    for PID in $TASK_PIDS; do
        echo -n "终止进程 $PID... "
        if kill -15 $PID 2>/dev/null; then
            echo "成功"
        else
            echo "失败"
            echo -n "尝试强制终止进程 $PID... "
            if kill -9 $PID 2>/dev/null; then
                echo "成功"
            else
                echo "失败"
            fi
        fi
    done
fi

# 查找并终止 ragflow_server.py 进程
echo ""
echo "查找 api/ragflow_server.py 进程..."
SERVER_PIDS=$(ps -ef | grep "python3.*api/ragflow_server.py" | grep -v grep | awk '{print $2}')

if [ -z "$SERVER_PIDS" ]; then
    echo "未找到 ragflow_server.py 进程"
else
    echo "找到以下 ragflow_server.py 进程:"
    for PID in $SERVER_PIDS; do
        CMDLINE=$(cat /proc/$PID/cmdline 2>/dev/null | tr '\0' ' ' || echo "无法读取")
        echo "PID: $PID - 命令: $CMDLINE"
    done

    echo "正在终止这些进程..."
    for PID in $SERVER_PIDS; do
        echo -n "终止进程 $PID... "
        if kill -15 $PID 2>/dev/null; then
            echo "成功"
        else
            echo "失败"
            echo -n "尝试强制终止进程 $PID... "
            if kill -9 $PID 2>/dev/null; then
                echo "成功"
            else
                echo "失败"
            fi
        fi
    done
fi

# 最后检查是否还有残留进程
echo ""
echo "检查是否有残留进程..."
REMAINING=$(ps -ef | grep -E "python3.*(rag/svr/task_executor.py|api/ragflow_server.py)" | grep -v grep)

if [ -z "$REMAINING" ]; then
    echo "所有进程已成功终止！"
else
    echo "仍有以下进程存在:"
    echo "$REMAINING"
    echo "可能需要手动终止这些进程"
fi
echo ""
echo "清理脚本执行完毕"