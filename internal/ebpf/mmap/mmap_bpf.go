package mmap

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -type mmap_event MmapProg ./mmap_bpf.c -- -O2 -g
