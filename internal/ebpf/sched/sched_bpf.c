// +build ignore

#include "vmlinux_wrapper.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

char LICENSE[] SEC("license") = "Dual BSD/GPL";

struct event {
    __u64 ts;
    __u32 type; // 1=exec, 2=exit
    __u32 pid;
    char comm[16];
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 18);  // 256KB instead of 16MB
} events SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u32);
} target_pid SEC(".maps");

const volatile struct event *unused_event __attribute__((unused));


static __always_inline int submit_event(struct event *e) {
    struct event *out = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!out) {
        return 0;
    }
    __builtin_memcpy(out, e, sizeof(*e));
    bpf_ringbuf_submit(out, 0);
    return 0;
}

SEC("tracepoint/sched/sched_process_exec")
int handle_exec(struct trace_event_raw_sched_process_exec *ctx) {
    struct event e = {};
    e.ts = bpf_ktime_get_tai_ns();
    e.type = 1;
    e.pid = ctx->pid;
    bpf_get_current_comm(&e.comm, sizeof(e.comm));
    return submit_event(&e);
}

SEC("tracepoint/sched/sched_process_exit")
int handle_exit(struct trace_event_raw_sched_process_template *ctx) {
    struct event e = {};
    e.ts = bpf_ktime_get_tai_ns();
    e.type = 2;
    e.pid = ctx->pid;
    bpf_get_current_comm(&e.comm, sizeof(e.comm));
    return submit_event(&e);
}


