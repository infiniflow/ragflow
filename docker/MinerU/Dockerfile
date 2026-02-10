# Use DaoCloud mirrored vllm image for China region for gpu with Ampere、Ada Lovelace、Hopper architecture (8.0 <= Compute Capability <= 9.0)
# Compute Capability version query (https://developer.nvidia.com/cuda-gpus)
# only support x86_64 architecture
#FROM docker.m.daocloud.io/vllm/vllm-openai:v0.10.1.1
FROM vllm-openai:v0.11.0


# Use DaoCloud mirrored vllm image for China region for gpu with Volta、Turing、Blackwell architecture (7.0 < Compute Capability < 8.0 or Compute Capability >= 10.0)
# support x86_64 architecture and ARM(AArch64) architecture
# FROM docker.m.daocloud.io/vllm/vllm-openai:v0.11.0

# Install libgl for opencv support & Noto fonts for Chinese characters
RUN apt-get update && \
    apt-get install -y \
        fonts-noto-core \
        fonts-noto-cjk \
        fontconfig \
        libgl1 && \
    fc-cache -fv && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install mineru latest
RUN python3 -m pip install -U 'mineru[core]>=2.7.0' -i https://mirrors.aliyun.com/pypi/simple --break-system-packages && \
    python3 -m pip cache purge

# Download models and update the configuration file
RUN /bin/bash -c "mineru-models-download -s modelscope -m all"

# Set the entry point to activate the virtual environment and run the command line tool
ENTRYPOINT ["/bin/bash", "-c", "export MINERU_MODEL_SOURCE=local && exec \"$@\"", "--"]