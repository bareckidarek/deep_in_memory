package sched

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -type event SchedProg ./sched_bpf.c -- -O2 -g
