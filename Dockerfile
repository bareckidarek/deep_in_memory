FROM golang:1.26.5 AS build

RUN apt-get update && \
    apt-get install -y bpftool clang llvm libbpf-dev && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /home/deep_in_memory

COPY ./cmd ./cmd
COPY ./internal ./internal
COPY ./go.mod ./
COPY ./go.sum ./

RUN bpftool btf dump file /sys/kernel/btf/vmlinux format c > vmlinux.h

RUN go generate ./... && CGO_ENABLED=0 go build -o deep_in_memory -tags ebpf ./cmd/deep_in_memory


FROM gcr.io/distroless/static-debian12

WORKDIR /tmp

COPY --from=build ./home/deep_in_memory/deep_in_memory /usr/local/bin/

CMD ["deep_in_memory"]