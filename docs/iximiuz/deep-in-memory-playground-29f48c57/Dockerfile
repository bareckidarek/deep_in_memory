FROM ghcr.io/iximiuz/labs/rootfs:ubuntu-docker

COPY *.c /home/laborant

RUN sudo apt update && sudo apt install -y gcc cgroup-tools

RUN for f in /home/laborant/*.c; do \
        gcc "$f" -o "${f%.c}"; \
    done