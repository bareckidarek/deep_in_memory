package paging

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target bpfel,bpfeb -type paging_event PagingProg ./paging_bpf.c -- -I../sched -Wall
