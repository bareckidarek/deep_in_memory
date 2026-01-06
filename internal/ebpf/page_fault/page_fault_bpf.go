package page_fault

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -type page_fault_event PageFaultProg ./page_fault_bpf.c -- -I../sched -Wall

