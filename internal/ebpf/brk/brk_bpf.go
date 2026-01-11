package brk

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -type brk_event BrkProg ./brk_bpf.c -- -O2 -g
