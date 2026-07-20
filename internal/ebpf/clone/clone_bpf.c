// +build ignore

#include "../../../vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

char LICENSE[] SEC("license") = "Dual BSD/GPL";

struct clone_event {
    __u64 ts;
    __u32 pid;
    __u32 tid;
    __u64 clone_flags;
    __u64 tls;
    char comm[16];
    };

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 18);  // 256KB instead of 16MB
} events SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 128);
    __type(key, __u32);
    __type(value, __u8);
} target_pids SEC(".maps");

const volatile struct clone_event *unused_event __attribute__((unused));

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 10240);
    __type(key, __u64);
    __type(value, struct clone_event);
} clone_entries SEC(".maps");


static __always_inline int submit_event(struct clone_event *e) {
    struct clone_event *out = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!out) {
        return 0;
    }
    __builtin_memcpy(out, e, sizeof(*e));
    bpf_ringbuf_submit(out, 0);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_clone")
int handle_clone_enter(struct trace_event_raw_sys_enter *ctx) {
    __u64 key = bpf_get_current_pid_tgid();
    __u32 pid = key >> 32;
    
    // Check target_pids filter - skip if no target set
    __u8 *exists = bpf_map_lookup_elem(&target_pids, &pid);
    if (!exists) {
        return 0; // No target set, skip
    }
    
    struct clone_event e = {};
    e.ts = bpf_ktime_get_tai_ns();
    e.pid = pid;
    e.tid = (__u32)key;
    e.clone_flags = (__u64)ctx->args[0]; // clone flags is first arg
    e.tls = (__u64)ctx->args[1]; // tls is second arg
    bpf_get_current_comm(&e.comm, sizeof(e.comm));
    
    bpf_map_update_elem(&clone_entries, &key, &e, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_clone")
int handle_clone_exit(struct trace_event_raw_sys_exit *ctx) {
    __u64 key = bpf_get_current_pid_tgid();

    struct clone_event *entry = bpf_map_lookup_elem(&clone_entries, &key);
    if (!entry) {
        return 0;
    }
    
    entry->tid = (__u32)ctx->ret;
    
    submit_event(entry);
    
    bpf_map_delete_elem(&clone_entries, &key);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_clone3")
int handle_clone3_enter(struct trace_event_raw_sys_enter *ctx) {
    __u64 key = bpf_get_current_pid_tgid();
    __u32 pid = key >> 32;

    __u8 *exists = bpf_map_lookup_elem(&target_pids, &pid);
    if (!exists) {
        return 0;
    }

    struct clone_args args = {};
    void *user_args = (void *)ctx->args[0];
    if (bpf_probe_read_user(&args, sizeof(args), user_args) != 0) {
        return 0;
    }

    struct clone_event e = {};
    e.ts = bpf_ktime_get_tai_ns();
    e.pid = pid;
    e.tid = (__u32)key;
    e.clone_flags = args.flags;
    e.tls = args.tls;
    bpf_get_current_comm(&e.comm, sizeof(e.comm));

    bpf_map_update_elem(&clone_entries, &key, &e, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_clone3")
int handle_clone3_exit(struct trace_event_raw_sys_exit *ctx) {
    __u64 key = bpf_get_current_pid_tgid();

    struct clone_event *entry = bpf_map_lookup_elem(&clone_entries, &key);
    if (!entry) {
        return 0;
    }

    entry->tid = (__u32)ctx->ret;

    submit_event(entry);

    bpf_map_delete_elem(&clone_entries, &key);
    return 0;
}
