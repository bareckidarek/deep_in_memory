package swap

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target bpfel,bpfeb -type swap_event SwapProg ./swap_bpf.c -- -I../sched -Wall
