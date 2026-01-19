// +build ignore

#include "vmlinux_wrapper.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

char LICENSE[] SEC("license") = "Dual BSD/GPL";

struct page_fault_event {
    __u64 ts;
    __u32 pid;
    __u32 tid;
    __u64 address;
    __u64 ip;
    __u64 error_code;
    char comm[16];
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 18);  // 256KB instead of 16MB
} page_fault_events SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u32);
} target_pid SEC(".maps");

const volatile struct page_fault_event *unused_event __attribute__((unused));

static __always_inline int submit_page_fault(struct page_fault_event *e) {
    struct page_fault_event *out = bpf_ringbuf_reserve(&page_fault_events, sizeof(*e), 0);
    if (!out) {
        return 0;
    }
    __builtin_memcpy(out, e, sizeof(*e));
    bpf_ringbuf_submit(out, 0);
    return 0;
}

// Raw tracepoint args for exceptions/page_fault_user
// Based on /sys/kernel/debug/tracing/events/exceptions/page_fault_user/format
struct page_fault_user_args {
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;
    
    unsigned long address;
    unsigned long ip;
    unsigned long error_code;
};

SEC("tracepoint/exceptions/page_fault_user")
int handle_page_fault_user(struct page_fault_user_args *ctx) {
    struct page_fault_event e = {};
    
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 pid = pid_tgid >> 32;
    
    // Check target_pid filter
    __u32 key = 0;
    __u32 *target = bpf_map_lookup_elem(&target_pid, &key);
    if (!target || *target == 0) {
        return 0; // No target set, skip
    }
    if (pid != *target) {
        return 0; // Not the target PID
    }
    
    e.ts = bpf_ktime_get_tai_ns();
    e.pid = pid;
    e.tid = (__u32)pid_tgid;
    e.address = ctx->address;
    e.ip = ctx->ip;
    e.error_code = ctx->error_code;
    bpf_get_current_comm(&e.comm, sizeof(e.comm));
    
    submit_page_fault(&e);
    return 0;
}
