package clone

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -type clone_event CloneProg ./clone_bpf.c -- -O2 -g
