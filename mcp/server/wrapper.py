#!/usr/bin/env python3
import sys
import os
import subprocess

# 记录所有参数
with open('/tmp/server_args.log', 'w') as f:
    f.write("Command arguments:\n")
    for i, arg in enumerate(sys.argv):
        f.write(f"Arg {i}: {arg}\n")
    f.write("\nEnvironment variables:\n")
    for key, value in os.environ.items():
        f.write(f"{key}={value}\n")

# 执行原始命令
original_script = sys.argv[1]
args = sys.argv[2:]
print(f"Executing: {original_script} with args: {args}")
subprocess.run([sys.executable, original_script] + args)
