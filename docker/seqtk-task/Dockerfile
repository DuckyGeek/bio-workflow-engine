# 使用国内的Ubuntu镜像源（清华镜像）
FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

# 使用清华镜像源加速apt-get
RUN sed -i 's/archive.ubuntu.com/mirrors.tuna.tsinghua.edu.cn/g' /etc/apt/sources.list && \
    sed -i 's/security.ubuntu.com/mirrors.tuna.tsinghua.edu.cn/g' /etc/apt/sources.list

RUN apt-get update && \
    apt-get install -y seqtk && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
COPY data/test.fasta /workspace/

CMD ["seqtk", "seq", "-A", "/workspace/test.fasta"]