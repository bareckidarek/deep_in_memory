.ONESHELL:
fmt:
	go fmt ./...

prepare:
	bpftool btf dump file /sys/kernel/btf/vmlinux format c > vmlinux.h

generate: prepare
	go generate ./internal/ebpf/brk
	go generate ./internal/ebpf/mmap
	go generate ./internal/ebpf/paging
	go generate ./internal/ebpf/sched

build:
	go build -o deep_in_memory -tags ebpf ./cmd/deep_in_memory

test:
	go test -v ./... -coverpkg=./... -coverprofile=coverage.out