.ONESHELL:
fmt:
	go fmt ./...

prepare:
	bpftool btf dump file /sys/kernel/btf/vmlinux format c > vmlinux.h

generate: prepare
	go generate ./...

build:
	go build -o deep_in_memory -tags ebpf ./cmd/deep_in_memory

test:
	go test -v ./... -coverpkg=./... -coverprofile=coverage.out