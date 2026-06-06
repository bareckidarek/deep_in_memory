// +build ignore

#include "../../../vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

char LICENSE[] SEC("license") = "Dual BSD/GPL";

struct mmap_event {
    __u64 ts;
    __u32 type; // 1=mmap, 2=munmap
    __u32 pid;
    __u32 tid;
    __u64 addr;
    __u64 len;
    __u64 ret;
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

const volatile struct mmap_event *unused_event __attribute__((unused));

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 10240);
    __type(key, __u64);
    __type(value, struct mmap_event);
} mmap_entries SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 10240);
    __type(key, __u64);
    __type(value, struct mmap_event);
} munmap_entries SEC(".maps");


static __always_inline int submit_event(struct mmap_event *e) {
    struct mmap_event *out = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!out) {
        return 0;
    }
    __builtin_memcpy(out, e, sizeof(*e));
    bpf_ringbuf_submit(out, 0);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_mmap")
int handle_mmap_enter(struct trace_event_raw_sys_enter *ctx) {
    __u64 key = bpf_get_current_pid_tgid();
    __u32 pid = key >> 32;
    
    // Check target_pid filter - skip if no target set
    __u32 map_key = 0;
    __u32 *target = bpf_map_lookup_elem(&target_pid, &map_key);
    if (!target || *target == 0) {
        return 0; // No target set, skip
    }
    if (pid != *target) {
        return 0; // Not the target PID
    }
    
    struct mmap_event e = {};
    e.ts = bpf_ktime_get_tai_ns();
    e.type = 1;
    e.pid = pid;
    e.tid = (__u32)key;
    e.len = (__u64)ctx->args[1]; // len is second arg
    bpf_get_current_comm(&e.comm, sizeof(e.comm));
    
    bpf_map_update_elem(&mmap_entries, &key, &e, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_mmap")
int handle_mmap_exit(struct trace_event_raw_sys_exit *ctx) {
    __u64 key = bpf_get_current_pid_tgid();
    
    struct mmap_event *entry = bpf_map_lookup_elem(&mmap_entries, &key);
    if (!entry) {
        return 0;
    }
    
    entry->addr = (__u64)ctx->ret;
    entry->ret = (__u64)ctx->ret;
    
    // Only submit if mmap succeeded (ret != -1 and ret != 0)
    if (ctx->ret > 0 && ctx->ret != (__s64)-1) {
        submit_event(entry);
    }
    
    bpf_map_delete_elem(&mmap_entries, &key);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_munmap")
int handle_munmap_enter(struct trace_event_raw_sys_enter *ctx) {
    __u64 key = bpf_get_current_pid_tgid();
    __u32 pid = key >> 32;
    
    // Check target_pid filter - skip if no target set
    __u32 map_key = 0;
    __u32 *target = bpf_map_lookup_elem(&target_pid, &map_key);
    if (!target || *target == 0) {
        return 0; // No target set, skip
    }
    if (pid != *target) {
        return 0; // Not the target PID
    }
    
    struct mmap_event e = {};
    e.ts = bpf_ktime_get_tai_ns();
    e.type = 2;
    e.pid = pid;
    e.tid = (__u32)key;
    e.addr = (__u64)ctx->args[0]; // addr is first arg
    e.len = (__u64)ctx->args[1];  // len is second arg
    bpf_get_current_comm(&e.comm, sizeof(e.comm));
    
    bpf_map_update_elem(&munmap_entries, &key, &e, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_munmap")
int handle_munmap_exit(struct trace_event_raw_sys_exit *ctx) {
    __u64 key = bpf_get_current_pid_tgid();
    
    struct mmap_event *entry = bpf_map_lookup_elem(&munmap_entries, &key);
    if (!entry) {
        return 0;
    }
    
    entry->ret = (__u64)ctx->ret;
    
    // Only submit if munmap succeeded (ret == 0)
    if (ctx->ret == 0) {
        submit_event(entry);
    }
    
    bpf_map_delete_elem(&munmap_entries, &key);
    return 0;
}
